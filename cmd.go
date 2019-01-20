package taskeeper

import (
	"log"
	"os"
)

//Command 执行命令的配置结构
//重新封装了cmd
type Command struct {
	id          string      //为每个命令随机分配一个字符串id
	name        string      //为命令指定一个名称
	pid         int         //命令如果运行 会将运行时的pid保存
	cmd         string      //命令的位置
	args        []string    //命令启动时的参数
	output      string      //命令执行时打印输出位置
	isCron      bool        //是否时定时任务
	cronExpress string      //定时任务表达式
	process     *os.Process //具体进程指针
}

//设置命令为cron命令
func (c *Command) SetCron(express string) *Command {
	c.isCron = true
	c.cronExpress = express
	return c
}

//设置命令的名称
func (c *Command) SetName(name string) *Command {
	c.name = name
	return c
}

//获取命令的名称
func (c *Command) Name() string {
	return c.name
}

//验证是否是cron命令
func (c *Command) IsCron() bool {
	return c.isCron
}

//获取命令输出打印位置
func (c *Command) Output() string {
	return c.output
}

//命令启动
func (c *Command) Start() int {
	var err error
	var file *os.File

	args := append([]string{c.cmd}, c.args...)
	file, _ = os.OpenFile(c.output, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0755)
	if file == nil {
		file = os.Stdout
	}
	c.process, err = os.StartProcess(c.cmd, args, &os.ProcAttr{Files: []*os.File{nil, file, file}})
	if err == nil {
		c.pid = c.process.Pid
		return c.pid
	}

	log.Println(c.cmd + " start failed : " + err.Error())
	return 0
}

//获取命令字符串Id 创建时随机分配
func (c *Command) ID() string {
	return c.id
}

//手动设置一个id
func (c *Command) SetId(id string) {
	c.id = id
}

//Pid 获取pid
func (c *Command) Pid() int {
	return c.pid
}

//重置命令pid 用于程序退出后标记
func (c *Command) ResetPid() {
	c.pid = 0
}

//Process 获取进程结构指针
func (c *Command) Process() *os.Process {
	return c.process
}

//Kill 杀死进程
func (c *Command) Kill() error {
	if c.process != nil {
		return c.process.Kill()
	}
	process, err := os.FindProcess(c.pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

//Wait 等待进程执行完毕
//进程结束后 会释放进程资源
func (c *Command) Wait() (*os.ProcessState, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Println(c.cmd + " wait() panic")
		}
	}()
	return c.process.Wait()
}

//Signal 向进程传递信号
func (c *Command) Singal(sig os.Signal) error {
	return c.process.Signal(sig)
}

//Release 释放进程资源
//释放以后 不能对进程进行任何操作
func (c *Command) Release() error {
	return c.process.Release()
}

//返回一个等待执行的cmd结构体
func NewCommand(cmd string, args []string, output string) *Command {
	return &Command{
		cmd:         cmd,
		args:        args,
		output:      output,
		isCron:      false,
		cronExpress: "",
	}
}
