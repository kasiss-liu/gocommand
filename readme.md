### taskeeper 进程管理器
类似supervisord的进程管理器

不需要环境支持，只要有命令文件即可，支持多平台

##### 功能设计
- 对常驻类型进程进行保活 
- 配置定时任务，定时启动脚本 (未完成)

##### 安装
```
$ mkdir ~/taskeeper
$ export GOPATH=~/taskeeper
$ go get -u github.com/kasiss-liu/taskeeper
$ go install github.com/kasiss-liu/taskeeper/keeper
$ go install github.com/kasiss-liu/taskeeper/keeperctl
```

##### 配置
```
# 使用yaml文件作为配置文件
cat github.com/kasiss-liu/taskeeper/config/config.yml
```
```
# 主程序日志打印位置 不需要保存日志可以配置为 `/dev/null`
log: ""           //如果配置项为空输出会打印到 stdout

# 服务启动时会开启一个tcp服务，接收管理客户端信号
host: ""          //默认主机 127.0.0.1 如果配置为空 将允许远程控制 否则需要删除host行
port: ""          //默认端口 17101

# 配置工作目录，如果程序运行时遇到相对路径，会以此项作为前缀补充为绝对路径  
workdir: ""

# 常驻进程异常中断重试次数 
# 如果在在5秒内 进程启动次数超过该配置，子命令将不再启动并标记失败
broken_gap: 10

# 命令列表
cmds:
 - 
  //子命令具体地址，建议配置为绝对路径 否则将根据workdir配置进行补充
  cmd: "test/test"
  //命令启动的参数
  args: 
   - "arg1"
   - "arg2"
  //该命令的输出打印位置 如果为空，将打印到主程序的输出位置  
  //如果为相对路径则会进行补充
  output: "test/test.log" 
 - 
  cmd: "/test/test"
  output: "/test/test.log"
  //如果该命令是定时任务 需要配置cron表达式
  //(暂未实现定时任务功能) 如果配置此项 该命令将不会启动 
  cron: "* * * * *"
```
##### 启动

```
keeper -c config.yml -d
```

#### 启动参数
```
keeper -h

Usage of keeper:
  -c string
    	config file in Yaml Format (default "config/config.yml")
  -d	is run in deamonize
  -flog
    	is force to print log
  -pprof
    	show runtime for testing
  -w string
    	keeper work absolute dir
```

#### 管理客户端

```
keeperctl 

Usage of keeperctl:
  -cat string
    	cat cmd status
  -h string
    	service hostname : 127.0.0.1
  -p string
    	service port : 17101
  -s string
    	ctl signal 'stop' , 'reload'
```

```
# 查看所有配置命令
keeperctl -cat cmdlist
# 查看单个命令运行状态 {cmdid前缀匹配}
keeperctl -cat cmd {cmdId} 
# 查看服务主进程状态 
keeperctl -cat status
# 重载配置
keeperctl -s reload 
# 停止服务
keeperctl -s stop 
```



