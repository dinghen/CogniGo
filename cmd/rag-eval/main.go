package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/dinghen/CogniGo/common/rag"
)

func main() {
	input := flag.String("input", "", "JSONL evaluation dataset")
	k := flag.Int("k", 5, "retrieval cutoff")
	flag.Parse()
	if *input == "" {
		fmt.Fprintln(os.Stderr, "usage: rag-eval -input dataset.jsonl [-k 5]")
		os.Exit(2)
	}
	file, err := os.Open(*input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer file.Close()
	result, err := rag.EvaluateJSONL(file, *k)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoded, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(encoded))
}
