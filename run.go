package taskeeper

import (
	"errors"
	"log"
	"sync"
	"time"
)

//状态机
type State struct {
	TasksNum   int
	RunningNum int
	BrokenNum  int

	RunningList  map[string]*Command
	BrokenList   map[string]*Command
	BrokenTries  map[string]int
	BrokenPoints map[string]int64
	Numlock      sync.Mutex
}

//运行时的必要参数
var (
	RunState    *State //状态机实例
	breakGap    int64  //异常中断容忍间隔
	brokenTimes = 5    //异常终端容忍次数
)

func init() {
	breakGap = DefaultBrokenGap
	RunState = &State{}
}

func Run() {

	err := savePid()
	//进程结束时 删除主进程和子进程的pid文件
	defer func() {
		delPidFile()
		delChildPidsFile()
	}()
	if err != nil {
		log.Println("run process prepare failed !")
		return
	}

	//初始化状态机实力参数
	initTask()
	//向通道内预写入一个开始信号
	go func() {
		signalChan <- sigStart
	}()
	//启动监控服务数据同步器
	syncStateToCopy()
	//按照配置启动命令
	for {
		sig := <-signalChan
		switch sig {
		//接收到所有进程异常信号后 退出主程序
		case sigBroken:
			//运行结束打印
			log.Println("run all process broken !")
			return
		//接收到重载信号后更新cmd配置 结束所有进程并按照新配置重新启动进程
		case sigReload:
			err := reloadTask()
			if err == nil {
				log.Println("run prepare reload process ...")
				exitTask()
				initTask()
				startTask()
				log.Println("run process reloaded !")
			}
		//接收到启动信号后 直接按照配置变量数据启动进程
		case sigStart:
			startTask()
		//接收到退出信号后 按次序杀死管理的进程 退出主程序
		case sigExit:
			log.Println("run starting exit process ...")
			exitTask()
			log.Println("run all process exit !")
			return
		default:
			log.Printf("undefined sig : %d \n", sig)
		}

	}

}

//验证是否所有协程都已经退出
//如果是 则向通道写入信号 通知主进程结束
func checkAllBroken() {
	if RunState.TasksNum > 0 && RunState.BrokenNum > 0 && RunState.TasksNum <= RunState.BrokenNum {
		signalChan <- sigBroken
	}
}

//运行常驻命令
func runDeamonRoutine(id string, c *Command) {
	RunState.BrokenTries[id] = 0
	for {
		//启动命令
		c.Start()
		//如果pid==0 则进程启动失败 该进程将不再重试
		if c.Pid() == 0 {
			RunState.Numlock.Lock()
			RunState.BrokenNum++
			RunState.Numlock.Unlock()
			RunState.BrokenList[id] = c
			break
		}
		//进程运行数+1
		RunState.Numlock.Lock()
		RunState.RunningNum++
		//将命令id 放入运行中的map
		RunState.RunningList[id] = c
		RunState.Numlock.Unlock()

		//等待程序运行结束
		_, err := c.Wait()
		//如果程序异常导致运行结束 打印异常退出原因
		if err != nil {
			log.Println("run routine except exit cmd:" + id + " errmsg:" + err.Error())
		}
		//验证是否是管理程序主动退出协程
		if _, ok := RunState.RunningList[id]; !ok {
			//log.Println("manager exit id:" + id)
			break
		}
		//从运行中状态机中移除本命令
		RunState.Numlock.Lock()
		RunState.RunningNum--
		delete(RunState.RunningList, id)
		RunState.Numlock.Unlock()

		//记录结束时间点
		brkTime := time.Now()
		//验证本命令是否曾经运行结束
		if ts, ok := RunState.BrokenPoints[id]; ok {
			//验证上次结束的时间与本次时间 间隔是否大于设定的间隔(s)
			if brkTime.Unix()-ts <= breakGap {
				//如果小于 则本命令的重试次数+1
				RunState.BrokenTries[id]++

				if RunState.BrokenTries[id] >= brokenTimes {
					//如果重试次数超限 则该进程存在异常 应该退出
					RunState.Numlock.Lock()
					RunState.BrokenNum++
					RunState.BrokenList[id] = c
					RunState.Numlock.Unlock()

					break
				}
				continue
			}
		}

		//如果大于设置的间隔或者还没有结束过 则看作是偶然退出 将错误次数设置为1
		//并更新时间点
		RunState.BrokenTries[id] = 1
		RunState.BrokenPoints[id] = brkTime.Unix()
	}
	//本协程结束时 验证状态机 是否所以协程都已经退出
	checkAllBroken()
}

//执行退出时，停止所有管理的进程
func exitTask() {
	for id, cmd := range RunState.RunningList {
		RunState.Numlock.Lock()
		RunState.RunningNum--
		delete(RunState.RunningList, id)
		RunState.Numlock.Unlock()
		err := cmd.Kill()
		if err != nil {
			log.Println("run kill error : " + err.Error())
		} else {
			log.Println("run kill cmd : " + id)
		}
	}
}

//重新读取配置 只更改cmd命令配置
func reloadTask() error {
	err := reloadConfigs()
	if err != nil {
		log.Println("run reload read config error : " + err.Error())
		return err
	}
	return nil
}

//初始化任务状态机
func initTask() {
	RunState = &State{}
	//启动命令时 初始化状态数据
	RunState.TasksNum = 0
	RunState.RunningNum = 0
	RunState.BrokenNum = 0
	RunState.RunningList = make(map[string]*Command)
	RunState.BrokenList = make(map[string]*Command)
	RunState.BrokenTries = make(map[string]int)
	RunState.BrokenPoints = make(map[string]int64)
}

//遍历启动所有cmd
func startTask() error {
	//服务已经启动过
	if RunState.TasksNum > 0 {
		startedErrmsg := "run start tasks has started"
		log.Println(startedErrmsg)
		return errors.New(startedErrmsg)
	}
	//启动服务
	for id, cmd := range cmds {
		if !cmd.IsCron() {
			RunState.TasksNum++
			go runDeamonRoutine(id, cmd)
			log.Println("run started cmd : " + id)
		}
	}
	//如果首次启动 记录启动时间
	if StartTime <= 0 {
		StartTime = time.Now().Unix()
	} else {
		//记录每次重载的时间
		ReloadTime = append(ReloadTime, time.Now().Unix())
	}
	return nil
}
