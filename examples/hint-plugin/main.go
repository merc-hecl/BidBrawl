// Build with: go build -o bin/quality-hint.exe ./examples/hint-plugin
// This example uses only the standard library and the documented JSON protocol.
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	var request struct {
		ProtocolVersion int `json:"protocol_version"`
		Items           []struct {
			Quality string `json:"quality"`
		} `json:"items"`
	}
	if json.NewDecoder(os.Stdin).Decode(&request) != nil || request.ProtocolVersion != 1 || len(request.Items) == 0 {
		os.Exit(1)
	}
	count := 0
	for _, item := range request.Items {
		if item.Quality == "common" {
			count++
		}
	}
	if json.NewEncoder(os.Stdout).Encode(map[string]any{
		"title": "普通品质数量", "message": fmt.Sprintf("箱内普通品质的物品共有 %d 件。", count),
		"fact": map[string]any{"type": "quality_count", "quality": "common", "count": count},
	}) != nil {
		os.Exit(1)
	}
}
