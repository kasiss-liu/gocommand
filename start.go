package taskeeper

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	loadConfig "github.com/kasiss-liu/go-tools/load-config"
)

const (
	//DefaultBrokenGap 默认的中断容忍间隔
	DefaultBrokenGap int64 = 5
	//DefaultHost 默认的tcp 主机地址
	DefaultHost = "127.0.0.1"
	//DefaultPort 默认的tcp 端口
	DefaultPort = "17101"
	//UnixSysRunDir uinx系的运行目录
	UnixSysRunDir = "/var/run/"
	//UnixSysTmpDir unix系的临时目录
	UnixSysTmpDir = "/tmp/"
)

var (
	//默认名称
	configName = "taskeeper"
	//tcp主机地址
	configHost string
	//tcp 启动端口 例如 127.0.0.1:17101
	configPort string
	//启动时载入的config文件结构
	configRaw *loadConfig.Config
	//主程序输出打印位置
	output = os.Stdout
	//存储config中配置的命令列表
	cmds map[string]*Command
	//自定义的容忍间隔
	customGap int64
	//.sock文件目录
	sockPath string
	//主程序打印位置
	logPath string
	//pid文件存储位置
	pidPath string
	//主程序启动的子程序pid存储位置
	cPidPath string
	//主程序启动的描述文件
	pidDescPath string
	//DefaultLogPath 默认主程序日志打印位置
	DefaultLogPath string
	//主程序工作目录
	workDir string
	//系统目录分隔符
	sysDirSep string
	//MainPid 主程序pid
	MainPid int
	//命令的名称对应id关系
	cmdNameMap map[string]string
	//AutoStart 自动启动命令
	AutoStart bool
)

//初始化命令map
func init() {
	cmds = make(map[string]*Command)
	cmdNameMap = make(map[string]string)
	//统一状态文件存放位置 保证多个程序读取到同一个pid文件
	//防止启动多个keeper导致管理混乱
	switch runtime.GOOS {
	case "windows":
		sockPath = os.Getenv("TEMP") + "\\taskeeper.sock"
		pidPath = os.Getenv("TEMP") + "\\taskeeper.pid"
		cPidPath = os.Getenv("TEMP") + "\\taskeeper.childs.pid"
		pidDescPath = os.Getenv("TEMP") + "\\taskeeper.pid.desc"
		DefaultLogPath = os.Getenv("TEMP") + "\\taskeeper.log"
	case "darwin", "linux":
		_, err := os.Stat(UnixSysRunDir)
		if os.IsNotExist(err) || os.IsPermission(err) {
			sockPath = UnixSysRunDir + "taskeeper.sock"
			pidPath = UnixSysRunDir + "taskeeper.pid"
			cPidPath = UnixSysRunDir + "taskeeper.childs.pid"
			pidDescPath = UnixSysRunDir + "taskeeper.pid.desc"
		} else {
			sockPath = UnixSysTmpDir + "taskeeper.sock"
			pidPath = UnixSysTmpDir + "taskeeper.pid"
			cPidPath = UnixSysTmpDir + "taskeeper.childs.pid"
			pidDescPath = UnixSysTmpDir + "taskeeper.pid.desc"
		}
		DefaultLogPath = "/tmp/taskeeper.log"
	}
	configHost = DefaultHost
	configPort = configHost + ":" + DefaultPort
	sysDirSep = string(os.PathSeparator)
	MainPid = os.Getpid()

}

//Start 启动服务
func Start(configPath string, deamon, forceLog bool) {

	log.SetOutput(output)
	//	if workDir == "" {
	//		SetWorkDir(GetParentDir(os.Args[0]),false)
	//	}
	//检查配置文件是否存在
	err := checkConfig(configPath)
	if err != nil {
		log.Fatalln("check config error : " + err.Error())
	}
	//检查pid文件
	err = checkPidFile()
	if err != nil {
		log.Fatalln("check pid file failed ")
	}
	//判断是否为后台进程运行模式
	//如果是 则由主进程启动一个子进程 运行命令
	if deamon {
		//判断当前进程是否为子进程 如果不是子进程 可以启动后台进程
		//Getppid 获取父进程进程id
		if os.Getppid() != 1 {
			cmdName := checkCommand(os.Args[0])
			cmd := NewCommand(cmdName, []string{"-c", configPath, "-flog", "-w", workDir}, "")
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
	err = readConfig(configPath)
	if err != nil {
		log.Println("read config error :" + err.Error())
	}
	//更改打印输出位置
	if len([]byte(logPath)) > 0 {
		//设置主输出
		err = setOutput(logPath)
		if err != nil {
			log.Println("set output error :" + err.Error())
		}
	} else {
		//如果这是后台模式启动的守护进程 且没有配置输出地址
		//将启用默认配置的日志路径
		if forceLog {
			err = setOutput(DefaultLogPath)
			if err != nil {
				log.Println("set output error :" + err.Error())
			}
			logPath = DefaultLogPath
		}
	}
	//如果配置文件有误 进程不能启动
	if err != nil {
		log.Println("check workdir : " + workDir)
		log.Fatalln("config file error : " + err.Error())
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
	if len(commands) > 0 {
		cmds = make(map[string]*Command)
		for _, cmdmap := range commands {
			cnf := loadConfig.BuildConfig(cmdmap)
			cmd, _ := cnf.Get("cmd").String()
			cmd = getAbsPath(cmd)
			output, _ := cnf.Get("output").String()
			output = getAbsPath(output)
			args, _ := cnf.Get("args").ArrayString()
			if len([]byte(cmd)) > 0 {
				c := NewCommand(cmd, args, output)
				cron, _ := cnf.Get("cron").String()
				name, _ := cnf.Get("name").String()

				if len([]byte(cron)) > 0 {
					c.SetCron(cron)
				}
				c.SetID(createID())

				//如果设置了命令的名称则使用 否则使用命令随机的id作为name
				if name != "" {
					c.SetName(name)
				} else {
					c.SetName(c.ID())
				}
				//注册命令名称到存储映射 方便查询
				cmdNameMap[c.Name()] = c.ID()

				cmds[c.ID()] = c
			}
		}
		return nil
	}
	return errors.New("no legal command registered")
}

//判断配置文件是否存在
func checkConfig(filename string) error {
	filename = getAbsPath(filename)
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
	filename = getAbsPath(filename)
	//读取原始内容
	configRaw, err = loadConfig.NewConfig(configName, filename)
	if err != nil {
		return err
	}
	//加载keeper进程的打印输出
	logPath, err = configRaw.Get("log").String()
	if err != nil {
		log.Println(err.Error())
	}
	//加载允许访问的host
	if !configRaw.Get("host").IsNil() {
		configHost, _ = configRaw.Get("host").String()
	}
	//加载工作目录
	if !configRaw.Get("workdir").IsNil() {
		wdir, _ := configRaw.Get("workdir").String()
		SetWorkDir(wdir)
	}
	//加载启用的端口
	if !configRaw.Get("port").IsNil() {
		if port, err := configRaw.Get("port").Int(); err == nil {
			configPort = configHost + ":" + strconv.Itoa(port)
		}
		if portStr, err := configRaw.Get("port").String(); err == nil && len(portStr) > 0 {
			configPort = configHost + ":" + portStr
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
			cmd = getAbsPath(cmd)
			output, _ := cnf.Get("output").String()
			output = getAbsPath(output)
			args, _ := cnf.Get("args").ArrayString()
			if len([]byte(cmd)) > 0 {
				c := NewCommand(cmd, args, output)

				cron, _ := cnf.Get("cron").String()
				if len([]byte(cron)) > 0 {
					c.SetCron(cron)
				}
				c.SetID(createID())

				name, _ := cnf.Get("name").String()
				//如果设置了命令的名称则使用 否则使用命令随机的id作为name
				if name != "" {
					c.SetName(name)
				} else {
					c.SetName(c.ID())
				}
				//注册命令名称到存储映射 方便查询
				cmdNameMap[c.Name()] = c.ID()

				cmds[c.ID()] = c

			}
		}
		return nil
	}
	return errors.New("no legal command registered")
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

//补充工作路径
func getAbsPath(p string) string {
	if !path.IsAbs(p) {
		p = workDir + sysDirSep + p
	}
	//	fmt.Println(p)
	return p
}

//随机数因子
//用以解决windows下出现的同一时刻
//会产生同一随机数的问题
var randSeed int64

//创建一个唯一id
func createID() string {
	for {
		rand.Seed(time.Now().UTC().Unix() + randSeed)
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
		randSeed++
		return result.String()
	}
}

//SetWorkDir 外部设置工作目录
//如果是绝对路径 直接赋值
//如果是相对路径 则按照当前目录为起始获取绝对路径
func SetWorkDir(dir string) bool {
	if dir == "" {
		return false
	}
	if filepath.IsAbs(dir) {
		workDir = dir
		return true
	}
	if abs, err := filepath.Abs(dir); err == nil {
		workDir = abs
		return true
	}
	return false
}
