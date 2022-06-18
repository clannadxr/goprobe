package kube

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"

	"github.com/gotomicro/ego/core/econf"
	"github.com/gotomicro/ego/core/elog"
	"go.uber.org/zap"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
	clientcmdapiv1 "k8s.io/client-go/tools/clientcmd/api/v1"
)

const (
	// High enough QPS to fit all expected use cases.
	defaultQPS = 1e6
	// High enough Burst to fit all expected use cases.
	defaultBurst = 1e6
)

var (
	clusterManagerSets = &sync.Map{} // dbClusterId [uint] -> *ClusterManager
)

type ClusterManager struct {
	Cluster *Cluster
	Client  *kubernetes.Clientset
	Config  *rest.Config
}

func InitApiServerClient() {
	newClusters, err := GetAllClusters()
	if err != nil {
		elog.Error("get all normal(status==0) clusters error while building apiServer client.", zap.Error(err))
		return
	}

	// build new clientManager
	for i := 0; i < len(newClusters); i++ {
		cluster := newClusters[i]
		// deal with invalid cluster
		if cluster.ApiServer == "" {
			elog.Warn("cluster's apiServer is null:%s", zap.String("clusterName", cluster.Name))
			continue
		}
		var buildOptions []ClientBuildOption
		if cluster.Proxy != "" {
			buildOptions = append(buildOptions, WithProxy(cluster.Proxy))
		}
		clientSet, config, err := buildClient(cluster.ApiServer, cluster.KubeConfig, buildOptions...)
		if err != nil {
			elog.Warn(fmt.Sprintf("build cluster (%s)'s client error.", cluster.Name), zap.Error(err))
			continue
		}

		clusterManager := &ClusterManager{
			Cluster: cluster,
			Client:  clientSet,
			Config:  config,
		}
		clusterManagerSets.Store(cluster.Name, clusterManager)
	}
	elog.Info("cluster finished! ")

}

func GetClusterManager(name string) (*ClusterManager, error) {
	managerInterface, exist := clusterManagerSets.Load(name)
	if !exist {
		return nil, fmt.Errorf("not exist name: " + name)
	}
	manager := managerInterface.(*ClusterManager)

	return manager, nil
}

type kubeClientOption struct {
	proxyAddr string
}
type ClientBuildOption func(*kubeClientOption)

func WithProxy(proxyAddr string) func(option *kubeClientOption) {
	return func(option *kubeClientOption) {
		option.proxyAddr = proxyAddr
	}
}

func buildClient(apiServerAddr string, kubeconfig string, options ...ClientBuildOption) (*kubernetes.Clientset, *rest.Config, error) {
	o := kubeClientOption{}
	for _, opt := range options {
		opt(&o)
	}
	configV1 := clientcmdapiv1.Config{}
	// 读取二进制内容
	rawData, err := ioutil.ReadFile(kubeconfig)
	if err != nil {
		return nil, nil, fmt.Errorf("buildClient 读取二进制失败, %w", err)
	}
	err = json.Unmarshal(rawData, &configV1)
	if err != nil {
		elog.Error("json unmarshal kubeconfig error.", zap.Error(err))
		return nil, nil, err
	}
	configObject, err := clientcmdlatest.Scheme.ConvertToVersion(&configV1, clientcmdapi.SchemeGroupVersion)
	configInternal := configObject.(*clientcmdapi.Config)

	clientConfig, err := clientcmd.NewDefaultClientConfig(*configInternal, &clientcmd.ConfigOverrides{
		ClusterDefaults: clientcmdapi.Cluster{Server: apiServerAddr}, // InsecureSkipTLSVerify: true,

	}).ClientConfig()

	if err != nil {
		elog.Error("build client config error. ", zap.Error(err))
		return nil, nil, err
	}

	clientConfig.QPS = defaultQPS
	clientConfig.Burst = defaultBurst

	if o.proxyAddr != "" {
		clientConfig.Proxy = func(request *http.Request) (*url.URL, error) {
			return url.Parse(o.proxyAddr)
		}
	}
	clientSet, err := kubernetes.NewForConfig(clientConfig)

	if err != nil {
		elog.Error(fmt.Sprintf("apiServerAddr(%s) kubernetes.NewForConfig(%v) error.", apiServerAddr, clientConfig), zap.Error(err))
		return nil, nil, err
	}

	return clientSet, clientConfig, nil
}

type Cluster struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ApiServer   string `json:"apiServer"`
	KubeConfig  string `json:"kubeConfig"`
	Proxy       string `json:"proxy"`
}

func GetAllClusters() (result []*Cluster, err error) {
	err = econf.UnmarshalKey("cluster", &result)
	return
}
