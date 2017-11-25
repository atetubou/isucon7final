package main

import (
	"log"
	"os"
	"os/exec"
	"runtime/pprof"
	"time"
)

var startLoggerToken = make(chan bool, 1)
var stopLoggerToken = make(chan bool, 1)
var BenchTime = 80

const LoggerBashScript = "/home/isucon/git/isucon7final/logger.sh"

func init() {
	startLoggerToken <- true
}

func ExecuteCommand(bashscript string) (string, error) {
	cmd := exec.Command("/bin/bash", "-s")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	stdin.Write([]byte(bashscript))
	stdin.Close()
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		return string(stdoutStderr), err
	}
	return string(stdoutStderr), nil
}
func MustExecuteCommand(bashscript string) string {
	res, err := ExecuteCommand(bashscript)
	if err != nil {
		log.Fatalf("Error while executing %s: %s", bashscript, res)
	}
	return res
}
func GetNextLogID() string {
	res := MustExecuteCommand(LoggerBashScript + " nextid")
	return res
}
func StartLogger(id string) { // id := GetNextID()
	// try to stop and wait until we get token
L:
	for {
		select {
		case <-startLoggerToken:
			break L
		case stopLoggerToken <- true:
		}
	}
	// clear stop token
	select {
	case <-stopLoggerToken:
	default:
	}

	// start logger
	log.Print(MustExecuteCommand(LoggerBashScript + " start " + id))
	f, err := os.Create("/tmp/cpu.prof")
	if err != nil {
		panic(err)
	}
	pprof.StartCPUProfile(f)

	log.Println("Started logger")

	// stop logger after 60 sec or stop logger token is placed
	go func(id string) {
		terminated := false
		select {
		case <-stopLoggerToken:
			terminated = true
		case <-time.After(time.Second * time.Duration(BenchTime)):
		}
		pprof.StopCPUProfile()
		err := f.Close()
		if err != nil {
			panic(err)
		}
		if terminated {
			res, err := ExecuteCommand(LoggerBashScript + " term " + id)
			log.Println(res)
			if err != nil {
				log.Println(err)
			}
		} else {
			log.Print(MustExecuteCommand(LoggerBashScript + " stop " + id))
		}
		log.Println("Stopped logger")
		startLoggerToken <- true
	}(id)
}
