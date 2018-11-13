package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/kasiss-liu/go-tools/load-config"
)

const (
	DefaultBrokenGap int64 = 5 //second
)

var (
	configName = "taskeeper"
	configPort = "127.0.0.1:6140"
	configRaw  *loadConfig.Config
	output     = os.Stdout
	cmds       map[string]*Command
	customGap  int64
	sockPath   string
	logPath    string
	pidPath    string
	cPidPath   string
)

//初始化命令map
func init() {
	cmds = make(map[string]*Command)
	sockPath = "run/taskeeper.sock"
	pidPath = "run/taskeeper.pid"
	cPidPath = "run/taskeeper_childs.pid"
}

func main() {
	//默认为前台运行 加参数-d 变为后台进程运行
	deamon := flag.Bool("d", false, "is run in deamonize")
	//配置文件路径 默认为config/config.yml
	config := flag.String("f", "config/config.yml", "config file in Yaml Format")

	//解析命令行参数
	flag.Parse()
	//设置打印位置 默认为系统stdout
	log.SetOutput(output)
	//检查配置文件是否存在
	err := checkConfig(*config)
	if err != nil {
		log.Fatalln(err.Error())
	}
	//检查pid文件
	err = checkPidFile()
	if err != nil {
		log.Fatalln("check pid file failed ")
	}
	//判断是否为后台进程运行模式
	//如果是 则由主进程启动一个子进程 运行命令
	if *deamon {
		//判断当前进程是否为子进程 如果不是子进程 可以启动后台进程
		//Getppid 获取父进程进程id
		if os.Getppid() != 1 {
			cmd := NewCommand(os.Args[0], []string{"-f", *config}, "")
			pid := cmd.Start()
			if pid > 0 {
				fmt.Printf("+[%d]\n", cmd.Pid())

			}
			return
		}
		fmt.Printf("%s\n", "process can not started by child process")
		return
	}
	//读取配置文件内容
	err = readConfig(*config)
	//如果配置文件有误 进程不能启动
	if err != nil {
		log.Fatalln(err)
	}
	//开启监听服务 接收管理客户端命令
	signal.Notify(sysSigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	startListenService()
	//监听系统信号
	listenSystemSig()
	//开始运行配置文件内注册的命令
	Run()
	//结束监听服务
	stopListenSerivce()

}

//读取配置文件内容 如果出错则会管理进程成
func reloadConfigs() error {
	cfgRaw, err := loadConfig.NewConfig(configName, configRaw.Path())

	if err != nil {
		return nil
	}
	//读取注册的命令 以及参数设置
	commands, err := cfgRaw.Get("cmds").Array()
	if err != nil {
		return err
	}
	tmpCmds := make(map[string]*Command)
	if len(commands) > 0 {
		for _, cmdmap := range commands {
			cnf := loadConfig.BuildConfig(cmdmap)
			cmd, _ := cnf.Get("cmd").String()
			output, _ := cnf.Get("output").String()
			args, _ := cnf.Get("args").ArrayString()
			if len([]byte(cmd)) > 0 {
				c := NewCommand(cmd, args, output)
				cron, _ := cnf.Get("cron").String()
				if len([]byte(cron)) > 0 {
					c.SetCron(cron)
				}
				c.SetId(createID())
				tmpCmds[c.ID()] = c
			}
		}
		cmds = tmpCmds
		return nil
	}
	return errors.New("No Legal Command Registered!")
}

//判断配置文件是否存在
func checkConfig(filename string) error {
	_, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

//读取配置文件内容
func readConfig(filename string) error {
	var err error
	//读取原始内容
	configRaw, err = loadConfig.NewConfig(configName, filename)
	if err != nil {
		return err
	}
	//加载keeper进程的打印输出
	logPath, err = configRaw.Get("log").String()
	if err != nil {
		log.Println(err.Error())
	} else {
		//设置主输出
		err = setOutput(logPath)
		if err != nil && logPath != "" {
			log.Println(err.Error())
		}
	}
	//加载容错时间
	brokenGap, err := configRaw.Get("broken_gap").Int()
	if err != nil {
		customGap = int64(0)
	} else {
		customGap = int64(brokenGap)
	}

	//读取注册的命令 以及参数设置
	commands, err := configRaw.Get("cmds").Array()
	if err != nil {
		return err
	}

	if len(commands) > 0 {
		for _, cmdmap := range commands {
			cnf := loadConfig.BuildConfig(cmdmap)
			cmd, _ := cnf.Get("cmd").String()
			output, _ := cnf.Get("output").String()
			args, _ := cnf.Get("args").ArrayString()
			if len([]byte(cmd)) > 0 {
				c := NewCommand(cmd, args, output)
				cron, _ := cnf.Get("cron").String()
				if len([]byte(cron)) > 0 {
					c.SetCron(cron)
				}
				c.SetId(createID())
				cmds[c.ID()] = c
			}
		}
		return nil
	}
	return errors.New("No Legal Command Registered!")
}

//更改输出打印位置
func setOutput(logPath string) (err error) {
	if len([]byte(logPath)) == 0 {
		return errors.New("log path empty ,output did not change")
	}
	var newOutput *os.File
	newOutput, err = os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0755)
	if err != nil {
		return err
	}
	log.SetOutput(newOutput)

	//关闭之前的打印接收资源
	if output != nil {
		output.Close()
	}
	output = newOutput
	return nil
}

//获取随机字符串
func getChar() string {

	rand.Seed(time.Now().UnixNano())
	switch rand.Intn(3) {
	case 0:
		return string(65 + rand.Intn(90-65))
	case 1:
		return string(97 + rand.Intn(122-97))
	default:
		return strconv.Itoa(rand.Intn(9))
	}
}
func createID() string {
	for {
		var result bytes.Buffer
		var temp string
		for i := 0; i < 10; {
			temp = getChar()
			result.WriteString(temp)
			i++
		}
		if _, ok := cmds[result.String()]; ok {
			continue
		}
		return result.String()
	}
}
