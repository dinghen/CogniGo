package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// WttrResponse is the subset of wttr.in's response used by the weather tool.
type WttrResponse struct {
	CurrentCondition []struct {
		TempC         string `json:"temp_C"`
		Humidity      string `json:"humidity"`
		WindspeedKmph string `json:"windspeedKmph"`
		WeatherDesc   []struct {
			Value string `json:"value"`
		} `json:"weatherDesc"`
	} `json:"current_condition"`
	NearestArea []struct {
		AreaName []struct {
			Value string `json:"value"`
		} `json:"areaName"`
	} `json:"nearest_area"`
}

// WeatherResponse is the stable structured response exposed by MCP.
type WeatherResponse struct {
	Location    string  `json:"location"`
	Temperature float64 `json:"temperature"`
	Condition   string  `json:"condition"`
	Humidity    int     `json:"humidity"`
	WindSpeed   float64 `json:"windSpeed"`
}

// WeatherAPIClient calls wttr.in.
type WeatherAPIClient struct {
	httpClient *http.Client
}

func NewWeatherAPIClient() *WeatherAPIClient {
	return &WeatherAPIClient{httpClient: http.DefaultClient}
}

func (c *WeatherAPIClient) GetWeather(ctx context.Context, city string) (*WeatherResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://wttr.in/%s?format=j1&lang=zh", url.PathEscape(city)), nil)
	if err != nil {
		return nil, fmt.Errorf("create weather request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request weather: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("weather service returned %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read weather response: %w", err)
	}
	var wttr WttrResponse
	if err := json.Unmarshal(body, &wttr); err != nil {
		return nil, fmt.Errorf("decode weather response: %w", err)
	}
	if len(wttr.CurrentCondition) == 0 {
		return nil, fmt.Errorf("weather service returned no current condition")
	}
	cc := wttr.CurrentCondition[0]
	temp, err := strconv.ParseFloat(cc.TempC, 64)
	if err != nil {
		return nil, fmt.Errorf("decode temperature: %w", err)
	}
	humidity, err := strconv.Atoi(cc.Humidity)
	if err != nil {
		return nil, fmt.Errorf("decode humidity: %w", err)
	}
	wind, err := strconv.ParseFloat(cc.WindspeedKmph, 64)
	if err != nil {
		return nil, fmt.Errorf("decode wind speed: %w", err)
	}
	location := city
	if len(wttr.NearestArea) > 0 && len(wttr.NearestArea[0].AreaName) > 0 {
		location = wttr.NearestArea[0].AreaName[0].Value
	}
	condition := "未知"
	if len(cc.WeatherDesc) > 0 {
		condition = cc.WeatherDesc[0].Value
	}
	return &WeatherResponse{Location: location, Temperature: temp, Condition: condition, Humidity: humidity, WindSpeed: wind}, nil
}

type weatherInput struct {
	City string `json:"city" jsonschema:"城市名称，如 Beijing、上海"`
}

// NewMCPServer creates the weather MCP server using the official SDK.
func NewMCPServer() *sdk.Server {
	server := sdk.NewServer(&sdk.Implementation{Name: "weather-query-server", Version: "1.0.0"}, nil)
	weatherClient := NewWeatherAPIClient()
	sdk.AddTool(server, &sdk.Tool{Name: "get_weather", Description: "获取指定城市的天气信息"}, func(ctx context.Context, _ *sdk.CallToolRequest, input weatherInput) (*sdk.CallToolResult, WeatherResponse, error) {
		if input.City == "" {
			return nil, WeatherResponse{}, fmt.Errorf("city is required")
		}
		weather, err := weatherClient.GetWeather(ctx, input.City)
		if err != nil {
			return nil, WeatherResponse{}, err
		}
		resultText := fmt.Sprintf(
			"城市: %s\n温度: %.1f°C\n天气: %s\n湿度: %d%%\n风速: %.1f km/h",
			weather.Location, weather.Temperature, weather.Condition, weather.Humidity, weather.WindSpeed,
		)
		return &sdk.CallToolResult{
			Content: []sdk.Content{&sdk.TextContent{Text: resultText}},
		}, *weather, nil
	})
	return server
}

// StartServer starts the Streamable HTTP MCP endpoint at /mcp.
func StartServer(httpAddr string) error {
	server := NewMCPServer()
	handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{})
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	return http.ListenAndServe(httpAddr, mux)
}
