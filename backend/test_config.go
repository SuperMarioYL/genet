package main

import (
	"encoding/json"
	"fmt"
	"github.com/uc-package/genet/internal/models"
)

func main() {
	config := models.DefaultConfig()
	
	response := models.ConfigResponse{
		PodLimitPerUser: config.PodLimitPerUser,
		GpuLimitPerUser: config.GpuLimitPerUser,
		GPUTypes:        config.GPU.AvailableTypes,
		PresetImages:    config.PresetImages,
		UI:              config.UI,
	}
	
	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	fmt.Println("配置 API 响应示例：")
	fmt.Println(string(jsonData))
}

