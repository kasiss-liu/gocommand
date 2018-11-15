package main

import (
	"flag"
	"log"
	"os"

	"github.com/kasiss-liu/taskeeper"
)

func main() {

	//默认为前台运行 加参数-d 变为后台进程运行
	deamon := flag.Bool("d", false, "is run in deamonize")
	//配置文件路径 默认为config/config.yml
	config := flag.String("c", "config/config.yml", "config file in Yaml Format")
	//工作目录 如果配置文件、脚本命令均被配置为相对路径 可以通过此配置设置相对路径起始位置
	workdir := flag.String("w", "", "keeper work absolute dir")
	//是否启用 日志强制打印
	forceLog := flag.Bool("flog", false, "is force to print log")

	//解析命令行参数
	flag.Parse()
	var setdir string
	if *workdir != "" {
		setdir = *workdir
	} else {
		setdir, _ = os.Getwd()
	}
	res := taskeeper.SetWorkDir(setdir)
	if !res && *workdir != "" {
		log.Println("workdir did not change")
	}

	taskeeper.Start(*config, *deamon, *forceLog)
}
