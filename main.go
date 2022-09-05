package main

import (
	"github.com/gotomicro/ego"
	"github.com/gotomicro/ego/core/elog"

	"goprobe/pkg/invoker"
	"goprobe/pkg/server"
)

func main() {
	err := ego.New().
		Invoker(invoker.Init).
		Serve(
			//egovernor.Load("server.governor").Build(),
			server.ServeHTTP(),
		).
		Run()
	if err != nil {
		elog.Panic("start up error: " + err.Error())
	}
}
