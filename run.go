package taskeeper

import (
	"errors"
	"log"
	"sync"
	"time"

	cron "github.com/kasiss-liu/gocrontab"
)

//State 状态机
type State struct {
	TasksNum   int //程序总的运行数量
	RunningNum int //正在运行的命令数
	BrokenNum  int //由于崩溃或结束运行的命令数

	RunningList  map[string]*Command //正在运行的命令map
	BrokenList   map[string]*Command //运行中断的命令map
	BrokenTries  map[string]int      //命令中断后在容忍间隔时间内的重试次数
	BrokenPoints map[string]int64    //命令中断的时间点
	Numlock      sync.Mutex          //操作各数量变更的锁

	CronState   bool                //是否已经开启cron协程
	SecCronList map[string]*Command //秒级cron列表
	MinCronList map[string]*Command //分钟级cron列表
	IsRun       bool                //是否已经开始运行
}

//运行时的必要参数
var (
	RunState    *State //状态机实例
	breakGap    int64  //异常中断容忍间隔
	brokenTimes = 5    //异常终端容忍次数
)

func init() {
	breakGap = DefaultBrokenGap         //初始化容忍间隔
	RunState = &State{CronState: false} //初始化一个状态机
}

//Run 程序启动
//子程序 启动、监控并等待操作信号
func Run() {

	//进程结束时 删除主进程和子进程的pid文件
	defer func() {
		delPidFile()
		delPidDescFile()
		delChildPidsFile()
	}()

	err := savePid()
	if err != nil {
		log.Println("run process prepare failed !")
		return
	}

	//初始化状态机实力参数
	initTask()
	//如果是自动运行模式 向通道内预写入一个开始信号
	if AutoStart {
		go func() {
			signalChan <- sigStart
		}()
	}
	//启动监控服务数据同步器
	syncStateToCopy()
	//按照配置启动命令
	for {
		sig := <-signalChan
		switch sig {
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
		case sigPause:
			log.Println("keeper pausing!")
			log.Println("run starting exit process ...")
			exitTask()
			log.Println("run all process exit !")
			log.Println("keeper paused!")
		case sigExit:
			log.Println("run starting exit process ...")
			exitTask()
			log.Println("run all process exit !")
			return
		//接收到单独控制命令
		case sigCtlCmd:
			doCtlCmdAction()
		default:
			log.Printf("undefined sig : %d \n", sig)
		}

	}

}

//运行常驻命令
func runDeamonRoutine(id string, c *Command) {
	//验证命令是否已经在运行
	if c.Pid() > 0 {
		log.Println("cmd:" + id + " is running")
		return
	}
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
			log.Println("cmd:" + id + " start failed no try")
			break
		}
		//进程运行数+1
		RunState.Numlock.Lock()
		RunState.RunningNum++
		//将命令id 放入运行中的map
		RunState.RunningList[id] = c
		RunState.Numlock.Unlock()

		//等待程序运行结束
		if c.Pid() > 0 {
			_, err := c.Wait()
			//如果程序异常导致运行结束 打印异常退出原因
			if err != nil {
				log.Println("run routine except exit cmd:" + id + " errmsg:" + err.Error())
			}
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
}

//执行退出时，停止所有管理的进程
func exitTask() {
	for id, cmd := range RunState.RunningList {
		exitSingleTask(id, cmd)
	}
	RunState.IsRun = false
}

//单个退出进程
func exitSingleTask(id string, cmd *Command) {
	RunState.Numlock.Lock()
	RunState.RunningNum--
	delete(RunState.RunningList, id)
	RunState.Numlock.Unlock()
	if cmd.Pid() > 0 {
		err := cmd.Kill()
		if err != nil {
			log.Println("run kill " + id + "error : " + err.Error())
		} else {
			log.Println("run kill cmd : " + id)
		}
	} else {
		log.Println("run kill cmd : " + id + "error: process not running")
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

	RunState.MinCronList = make(map[string]*Command)
	RunState.SecCronList = make(map[string]*Command)
}

//遍历启动所有cmd
func startTask() error {
	//服务已经启动过
	if RunState.IsRun {
		startedErrmsg := "run start tasks has started"
		log.Println(startedErrmsg)
		return errors.New(startedErrmsg)
	}

	RunState.IsRun = true

	//启动服务
	for id, cmd := range cmds {
		if !cmd.IsCron() {
			RunState.TasksNum++
			if !cmd.IsPause() {
				go runDeamonRoutine(id, cmd)
				log.Println("run started cmd : " + id)
			} else {
				log.Println("run paused cmd : " + id)
			}

		}
		if cmd.IsCron() {
			checkCronExpress(cmd)
			if !RunState.CronState {
				go runSecondCronRoutine()
				go runMinuteCronRoutine()
				RunState.CronState = true
			}
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

//单独重启一个执行命令
func restartTask(cid string) {
	for id, cmd := range cmds {
		if id == cid {
			//启动命令
			if !cmd.IsCron() {
				//终结之前运行的进程
				exitSingleTask(id, cmd)
				//开启新进程
				if !cmd.IsPause() {
					go runDeamonRoutine(id, cmd)
					log.Println("run started cmd : " + id)
					return
				}
				log.Println("run restart error: Cmd is Paused -- " + id)
				return
			}
			log.Println("run restart error: Cmd in Cron Type cannot be run -- " + id)
			return
		}
	}
	log.Println("run restart error : No cmd found -- " + cid)
	return
}

//启动秒级cron运行任务
func runSecondCronRoutine() {
	for {
		for _, cmd := range RunState.SecCronList {
			if cron.ValidExpressNow(cmd.cronExpress) {
				if !cmd.IsPause() {
					log.Println("cron sec " + cmd.ID())
					go doCronRoutine(cmd)
				} else {
					log.Println("cron sec " + cmd.ID() + "is paused")
				}
			}
		}
		time.Sleep(1 * time.Second)
	}
}

//启动分钟级cron运行任务
func runMinuteCronRoutine() {
	for {
		for _, cmd := range RunState.MinCronList {
			if cron.ValidExpressNow(cmd.cronExpress) {
				if !cmd.IsPause() {
					log.Println("cron min " + cmd.ID())
					go doCronRoutine(cmd)
				} else {
					log.Println("cron min " + cmd.ID() + "is paused")
				}
			}
		}
		time.Sleep(60 * time.Second)
	}
}

//解析cron
func checkCronExpress(cmd *Command) {
	if !cmd.IsCron() {
		log.Println("min cron" + cmd.ID())
		return
	}
	c, err := cron.NewCronWithExpress(cmd.cronExpress)
	if err != nil {
		log.Println(`cron express error: ` + err.Error())
		return
	}
	if c.IsSec() {
		RunState.SecCronList[cmd.id] = cmd
	} else {
		RunState.MinCronList[cmd.id] = cmd
	}
	RunState.TasksNum++
}

//协程启动cron进程
func doCronRoutine(cmd *Command) {
	if cmd.Pid() > 0 {
		log.Println("cron ignore " + cmd.ID())
		return
	}

	cmd.Start()
	if cmd.Pid() > 0 {
		log.Println("cron cmd id: " + cmd.ID() + " started")
		_, err := cmd.Wait()
		if err != nil {
			log.Println("cron cmd id: " + cmd.ID() + " msg:" + err.Error())
		}
	}

	cmd.pid = -1
}

//单独处理命令操作
func doCtlCmdAction() {
	act := <-signalCmdCtlChan
	switch act.sig {
	case sigExit:
		cid, ok := findCmdIDByName(act.cmdid)
		if ok {
			if cmd, ok := cmds[cid]; ok {
				go exitSingleTask(cid, cmd)
				return
			}
		}
		log.Println("ctl action exit error : no cmd found -- " + act.cmdid)
	case sigExec:
		cid, ok := findCmdIDByName(act.cmdid)
		if ok {
			if cmd, ok := cmds[cid]; ok {
				if cmd.isCron {
					go runDeamonRoutine(cid, cmd)
				} else {
					go doCronRoutine(cmd)
				}
				break
			}
		}
		log.Println("ctl action exit error : no cmd found -- " + act.cmdid)
	case sigReload:
		cid, ok := findCmdIDByName(act.cmdid)
		if ok {
			go restartTask(cid)
		} else {
			log.Println("ctl action reload error : no cmd found -- " + act.cmdid)
		}
	case sigStart:
		cid, ok := findCmdIDByName(act.cmdid)
		if ok {
			if cmd, ok := cmds[cid]; ok {
				cmd.SetRun()
				if !cmd.isCron {
					go runDeamonRoutine(cid, cmd)
				}
				break
			}
		}
		log.Println("ctl action setRun error : no cmd found -- " + act.cmdid)
	case sigPause:
		cid, ok := findCmdIDByName(act.cmdid)
		if ok {
			if cmd, ok := cmds[cid]; ok {
				cmd.SetPause()
				if !cmd.isCron {
					go exitSingleTask(cid, cmd)
				}
				break
			}
		}
		log.Println("ctl action setRun error : no cmd found -- " + act.cmdid)
	default:
	}
}
