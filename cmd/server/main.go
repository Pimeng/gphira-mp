package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"phira-mp/server"
)

func main() {
	// 解析命令行参数
	port := flag.Int("port", 12346, "服务器端口")
	flag.Parse()

	// 加载配置
	config, err := server.LoadConfig("server_config.yml")
	if err != nil {
		log.Printf("加载配置失败: %v, 使用默认配置", err)
		config = server.DefaultConfig()
	}

	// 创建服务器
	srv := server.NewServer(config)

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动服务器
	go func() {
		address := fmt.Sprintf(":%d", *port)
		if err := srv.Start(address); err != nil {
			log.Fatalf("服务器错误: %v", err)
		}
	}()

	log.Printf("Phira-MP 服务器已启动，端口: %d", *port)
	log.Println("按 Ctrl+C 停止服务器")

	// 等待信号
	<-sigChan
	log.Println("正在关闭服务器...")
	srv.Stop()
	log.Println("服务器已停止")
}
