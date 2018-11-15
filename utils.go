package taskeeper

import (
	"encoding/json"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"strings"
)

//获取父级目录地址
func GetParentDir(p string) string {
	p = strings.Replace(p, "\\", sysDirSep, -1)
	p = strings.Replace(p, "/", sysDirSep, -1)
	parr := strings.Split(p, sysDirSep)
	return strings.Join(parr[:len(parr)-1], sysDirSep)
}

//获取随机字符串
func getChar() string {
	switch rand.Intn(3) {
	case 0:
		return string(65 + rand.Intn(90-65))
	case 1:
		return string(97 + rand.Intn(122-97))
	default:
		return strconv.Itoa(rand.Intn(9))
	}
}

func GetPidFile() string {
	return pidPath
}

func GetChildPidsFile() string {
	return cPidPath
}

func GetTcpAddr() string {
	return configPort
}

func ParsePidDesc() (ProcessConfig, error) {
	p := ProcessConfig{}
	_, err := os.Stat(pidDescPath)
	if os.IsNotExist(err) {
		return p, err
	}
	data, err := ioutil.ReadFile(pidDescPath)
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(data, &p)
	if err != nil {
		return p, err
	}
	return p, nil
}
