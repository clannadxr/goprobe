package invoker

import (
	"goprobe/pkg/kube"
	"goprobe/pkg/pprof"
)

func Init() error {
	initKubeClient()
	err := pprof.Init()
	if err != nil {
		return err
	}
	return nil
}

func initKubeClient() {
	kube.InitApiServerClient()
}
