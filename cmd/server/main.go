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
	host := flag.String("host", "", "服务器监听地址（留空则使用配置文件）")
	port := flag.Int("port", 0, "服务器端口（0则使用配置文件）")
	flag.Parse()

	// 加载配置
	config, err := server.LoadConfig("server_config.yml")
	if err != nil {
		log.Printf("加载配置失败: %v, 使用默认配置", err)
		config = server.DefaultConfig()
	}

	// 命令行参数优先级高于配置文件
	if *host != "" {
		config.Host = *host
	}
	if *port != 0 {
		config.Port = *port
	}

	// 创建服务器
	srv := server.NewServer(config)

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动服务器
	go func() {
		// 构建监听地址，处理IPv6格式
		var address string
		if config.Host == "" || config.Host == "0.0.0.0" {
			// 空或IPv4格式，直接使用
			address = fmt.Sprintf("%s:%d", config.Host, config.Port)
		} else if config.Host == "::" {
			// IPv6通配符地址
			address = fmt.Sprintf("[::]:%d", config.Port)
		} else if len(config.Host) > 0 && config.Host[0] == ':' && config.Host != "::" {
			// 已经是 :port 格式
			address = fmt.Sprintf("%s%d", config.Host, config.Port)
		} else {
			// 其他情况（包括具体IPv6地址）
			address = fmt.Sprintf("%s:%d", config.Host, config.Port)
		}
		
		if err := srv.Start(address); err != nil {
			log.Fatalf("服务器错误: %v", err)
		}
	}()

	// 格式化显示地址
	displayAddr := config.Host
	if config.Host == "" {
		displayAddr = "0.0.0.0"
	}
	log.Printf("Phira-MP 服务器已启动，地址: %s:%d", displayAddr, config.Port)
	log.Println("按 Ctrl+C 停止服务器")

	// 等待信号
	<-sigChan
	log.Println("正在关闭服务器...")
	srv.Stop()
	log.Println("服务器已停止")
}
