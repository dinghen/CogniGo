package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	mcpclient "github.com/dinghen/CogniGo/common/mcp/client"
	mcpserver "github.com/dinghen/CogniGo/common/mcp/server"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	// 定义命令行标志
	mode := flag.String("mode", "", "运行模式: server 或 client")
	httpAddr := flag.String("http-addr", ":8081", "HTTP服务器地址")
	city := flag.String("city", "", "要查询天气的城市名称")
	flag.Parse()

	if *mode == "" {
		fmt.Println("Error: 您必须指定模式使用--mode (server 或 client)")
		flag.Usage()
		os.Exit(1)
	}

	if *mode == "server" {
		// 启动服务器
		fmt.Println("启动MCP服务器...")
		if err := mcpserver.StartServer(*httpAddr); err != nil {
			log.Fatalf("服务器错误: %v", err)
		}
	} else if *mode == "client" {
		// 运行客户端
		if *city == "" {
			fmt.Println("Error: 您必须指定城市名称使用--city")
			flag.Usage()
			os.Exit(1)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// 创建客户端并完成 Initialize
		httpURL := "http://localhost:8081/mcp"
		mcpClient, err := mcpclient.NewMCPClient(ctx, httpURL)
		if err != nil {
			log.Fatalf("创建客户端失败: %v", err)
		}
		defer mcpClient.Close()

		// 执行健康检查
		if err := mcpClient.Ping(ctx); err != nil {
			log.Fatalf("健康检查失败: %v", err)
		}

		// 动态发现工具后调用天气工具
		tools, err := mcpClient.ListTools(ctx)
		if err != nil {
			log.Fatalf("列出工具失败: %v", err)
		}
		found := false
		for _, tool := range tools {
			if tool.Name == "get_weather" {
				found = true
				break
			}
		}
		if !found {
			log.Fatalf("MCP 服务未提供 get_weather 工具")
		}
		result, err := mcpClient.CallTool(ctx, "get_weather", map[string]any{"city": *city})
		if err != nil {
			log.Fatalf("调用工具失败: %v", err)
		}

		// 显示天气结果
		fmt.Println("\n天气查询结果:")
		for _, content := range result.Content {
			if text, ok := content.(*sdk.TextContent); ok {
				fmt.Println(text.Text)
			}
		}

		fmt.Println("\n客户端初始化成功。正在关闭...")
	}
}
