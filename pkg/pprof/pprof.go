package pprof

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gotomicro/ego/core/econf"
	"github.com/gotomicro/ego/core/elog"
	"github.com/pkg/errors"
	"github.com/spf13/cast"
	torchPprof "github.com/uber-archive/go-torch/pprof"
	"github.com/uber-archive/go-torch/renderer"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"goprobe/pkg/dto"
	"goprobe/pkg/kube"
	"goprobe/pkg/storage"
	"goprobe/pkg/storage/filesystem"
)

var profileTypes = []string{"block", "goroutine", "heap", "profile"}

const (
	ProfileRunTypePod  = "pod"
	ProfileRunTypeAddr = "ip"
)

var Pprof *pprof

func Init() error {
	// check path
	basePath := econf.GetString("storage.filesystem.basePath")
	err := os.MkdirAll(basePath, 0755)
	if err != nil {
		return fmt.Errorf("init pprof check dir failed: %w", err)
	}
	Pprof = &pprof{
		storage: filesystem.NewFilesystemClient(basePath),
	}
	err = Pprof.checkEnv()
	if err != nil {
		return fmt.Errorf("init pprof check env failed: %w", err)
	}
	return nil
}

type pprof struct {
	storage storage.Client
}

type PprofInfo struct {
	Type string `json:"type"`
	Url  string `json:"url"`
}

// GeneratePprof 生成PProf图
func (p *pprof) GeneratePprof(reqRunProfile dto.ReqRunProfile) (list []PprofInfo, err error) {
	list = make([]PprofInfo, 0)
	switch reqRunProfile.Mode {
	case ProfileRunTypePod:
		if reqRunProfile.PodName == "" || reqRunProfile.ClusterName == "" {
			err = fmt.Errorf("pod_name or cluster_id cannot be empty")
			return
		}
		reqRunProfile.UniqueKey = fmt.Sprintf("%s/%s/%s_%d", reqRunProfile.ClusterName, reqRunProfile.Namespace, reqRunProfile.PodName, time.Now().UnixMilli())
		if reqRunProfile.Port == 0 {
			err = fmt.Errorf("治理端口未设置，请设置治理端口")
			return
		}
		var targetClusterManager *kube.ClusterManager
		targetClusterManager, err = kube.GetClusterManager(reqRunProfile.ClusterName)
		if err != nil {
			elog.Error("Get clusterManager failed while gen pprof.",
				zap.String("requestClusterId", reqRunProfile.ClusterName), zap.Error(err))
			err = fmt.Errorf("target cluster may not exist, please retry")
			return
		}

		eg := errgroup.Group{}
		for _, _profileType := range profileTypes {
			profileType := _profileType
			eg.Go(func() error {
				params := make(map[string]string)
				if profileType == "profile" {
					params["seconds"] = strconv.Itoa(reqRunProfile.Seconds)
				}

				err = p.generateGraphByK8S(reqRunProfile, targetClusterManager, profileType, params)
				if err != nil {
					return err
				}
				list = append(list, PprofInfo{
					Type: profileType,
					Url:  getPprofUrl(profileType, reqRunProfile.UniqueKey, "flame"),
				})
				list = append(list, PprofInfo{
					Type: profileType,
					Url:  getPprofUrl(profileType, reqRunProfile.UniqueKey, "profile"),
				})
				return nil
			})
		}
		err = eg.Wait()
		if err != nil {
			return
		}
		return
	case ProfileRunTypeAddr:
		if reqRunProfile.Addr == "" {
			err = errors.New("addr cannot be empty")
			return
		}
		reqRunProfile.UniqueKey = fmt.Sprintf("%s/%s/%s_%d", reqRunProfile.ClusterName, "custom", reqRunProfile.Addr, time.Now().UnixMilli())
		eg := errgroup.Group{}
		for _, _profileType := range profileTypes {
			profileType := _profileType
			eg.Go(func() error {
				params := make(map[string]string)
				if profileType == "profile" {
					params["seconds"] = strconv.Itoa(reqRunProfile.Seconds)
				}
				elog.Info("pprof", elog.String("profileType", profileType), elog.Any("reqRunProfile", reqRunProfile))
				err = p.generateGraphByAddr(reqRunProfile, profileType, params)
				if err != nil {
					return err
				}
				list = append(list, PprofInfo{
					Type: profileType,
					Url:  getPprofUrl(profileType, reqRunProfile.UniqueKey, "flame"),
				})
				list = append(list, PprofInfo{
					Type: profileType,
					Url:  getPprofUrl(profileType, reqRunProfile.UniqueKey, "profile"),
				})
				return nil
			})
		}

		err = eg.Wait()
		if err != nil {
			return
		}
		return
	default:
		err = fmt.Errorf("ProfileRunType (%s) isn't supported currently", reqRunProfile.Mode)
		return
	}

}
func (p *pprof) FindGraphData(req dto.ReqPprofGraph) (data []byte, err error) {
	svgPath := filepath.Join(req.Url, req.GoType+"_"+req.SvgType+".svg")
	// SVG
	switch req.SvgType {
	case "profile":
		fallthrough
	case "flame":
		data, err = p.storage.GetBytes(context.TODO(), svgPath)
	default:
		return nil, fmt.Errorf("no exist svg type: " + req.SvgType)
	}
	return
}

func (p *pprof) GetPprofList(req dto.ReqGetPprofList) (list []dto.RespGetPprofListItem, err error) {
	key := fmt.Sprintf("%s/%s", req.ClusterName, req.Namespace)
	nameList, err := p.storage.List(context.TODO(), key)
	if err != nil {
		return nil, err
	}
	for _, item := range nameList {
		n := strings.LastIndex(item, "_")
		list = append(list, dto.RespGetPprofListItem{
			Url:     fmt.Sprintf("%s/%s", key, item),
			PodName: item[:n],
			Ctime:   cast.ToInt64(item[n+1:]) / 1e3,
		})
	}
	return
}

func (p *pprof) checkEnv() (err error) {
	// 1 check go version
	if _, err = exec.Command("go", "version").Output(); err != nil {
		return fmt.Errorf("there was an error running 'go version' command: %s", err)
	}

	// 2 check dot -v, graphiz
	if _, err = exec.Command("dot", "-v").Output(); err != nil {
		return fmt.Errorf("there was an error running 'dot -v' command: %s", err)
	}

	// 3 check flamegraph.pl
	flameGraphScripts := []string{"flamegraph", "flamegraph.pl", "./flamegraph.pl", "./FlameGraph/flamegraph.pl", "flame-graph-gen"}
	var b bool
	for _, v := range flameGraphScripts {
		if _, err := exec.LookPath(v); err == nil {
			b = true
			break
		}
	}
	if !b {
		return errors.New("flameGraphScript not found")
	}
	return
}

func (p *pprof) generateGraphByAddr(reqRunProfile dto.ReqRunProfile, pprofResName string, params map[string]string) (err error) {
	targetUrl := fmt.Sprintf("%s/debug/pprof/%s", reqRunProfile.Addr, pprofResName)
	if pprofResName == "fgprof" {
		targetUrl = fmt.Sprintf("%s/debug/%s", reqRunProfile.Addr, pprofResName)
	}
	if !strings.HasPrefix(targetUrl, "http://") || !strings.HasPrefix(targetUrl, "https://") ||
		!strings.HasPrefix(targetUrl, "/") || !strings.HasPrefix(targetUrl, "//") {
		targetUrl = "http://" + targetUrl
	}
	timeout := 5 * time.Second
	if _, exist := params["seconds"]; exist {
		if secs, err := strconv.Atoi(params["seconds"]); err == nil && secs > 0 {
			timeout = time.Duration(secs+5) * time.Second
		}
	}
	c := &http.Client{Timeout: timeout} // default timeout 5s
	elog.Info("pprof", elog.String("targetUrl", targetUrl), elog.String("pprofResName", pprofResName), zap.Duration("timeout", timeout))

	req, err := http.NewRequest("GET", targetUrl, nil)
	if err != nil {
		return err
	}
	q := req.URL.Query()
	for key, val := range params {
		q.Set(key, val)
	}
	req.URL.RawQuery = q.Encode()
	res, err := c.Do(req)
	if err != nil {
		err = errors.Wrapf(err, "请求地址(%s)获取 %s profile 数据失败. err=%s", targetUrl, pprofResName, err.Error())
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		err = errors.Errorf("请求地址(%s)获取 %s profile 数据失败: statusCode is %d", targetUrl, pprofResName, res.StatusCode)
		return
	}

	rawProfileData, err := ioutil.ReadAll(res.Body)
	if err != nil {
		err = errors.Wrapf(err, "请求地址(%s)获取 %s profile response数据失败. err=%s", targetUrl, pprofResName, err.Error())
		return
	}

	err = p.genSvg(rawProfileData, reqRunProfile.UniqueKey, pprofResName)
	if err != nil {
		err = fmt.Errorf("generateGraphByAddr err: %w", err)
		return
	}
	return
}

func (p *pprof) generateGraphByK8S(reqRunProfile dto.ReqRunProfile, clusterManager *kube.ClusterManager,
	pprofResName string, params map[string]string) (err error) {

	resourceName := fmt.Sprintf("%s:%d", reqRunProfile.PodName, reqRunProfile.Port)
	suffix := "debug/pprof/" + pprofResName
	if pprofResName == "fgprof" {
		suffix = "debug/" + pprofResName
	}
	elog.Info("pprof", elog.String("suffix", suffix))
	req := clusterManager.Client.CoreV1().RESTClient().
		Get().
		Namespace(reqRunProfile.Namespace).
		Resource("pods").
		Name(resourceName).
		SubResource("proxy").
		Suffix(suffix)

	for key, val := range params {
		req = req.Param(key, val)
	}

	res := req.Do(context.Background())
	err = res.Error()
	if err != nil {
		err = errors.Wrapf(err, "请求治理端口获取 %s profile 数据失败. err=%s", pprofResName, err.Error())
		return
	}
	rawProfileData, _ := res.Raw()
	err = p.genSvg(rawProfileData, reqRunProfile.UniqueKey, pprofResName)
	if err != nil {
		err = fmt.Errorf("generateGraphByK8S err: %w", err)
		return
	}
	return
}

func (p *pprof) genSvg(rawProfileData []byte, uniqueKey string, pprofType string) (err error) {
	tmpFileDir := filepath.Join(os.TempDir(), uniqueKey)
	err = os.MkdirAll(tmpFileDir, os.ModePerm)
	if err != nil {
		err = errors.Wrap(err, "创建临时目录失败")
		return
	}

	rawStorePath := path.Join(tmpFileDir, pprofType+".bin")
	err = ioutil.WriteFile(rawStorePath, rawProfileData, os.ModePerm)
	if err != nil {
		err = errors.Wrap(err, "临时文件写入失败")
		return
	}
	// 保存 bin 文件
	err = p.storage.PutBytes(context.TODO(), filepath.Join(uniqueKey, pprofType+".bin"), rawProfileData)
	if err != nil {
		err = errors.Wrap(err, "临时文件保存失败")
		return
	}

	var (
		flameSvgByte   []byte
		profileSvgByte []byte
	)

	// 生成火焰图 SVG
	flameSvgByte, err = p.generateFlameSvg(rawStorePath)
	if err != nil {
		err = fmt.Errorf("生成火焰图失败, %w", err)
		return
	}

	err = p.storage.PutBytes(context.TODO(), filepath.Join(uniqueKey, pprofType+"_flame.svg"), flameSvgByte)
	if err != nil {
		err = fmt.Errorf("保存火焰图失败: %w", err)
		return
	}

	// 生成Profile SVG
	profileSvgPath := path.Join(tmpFileDir, pprofType+"_profile.svg")
	profileSvgByte, err = p.generateProfileSvg(rawStorePath, profileSvgPath)
	if err != nil {
		err = fmt.Errorf("生成Profile图失败, %w", err)
		return
	}

	err = p.storage.PutBytes(context.TODO(), filepath.Join(uniqueKey, pprofType+"_profile.svg"), profileSvgByte)
	if err != nil {
		err = fmt.Errorf("保存 Profile 图失败: %w", err)
	}
	return nil
}

// 生成火焰图SVG
func (p *pprof) generateFlameSvg(rawFilePath string) (data []byte, err error) {
	out, err := exec.Command("bash", "-c", "go tool pprof -raw "+rawFilePath).Output()
	if err != nil {
		return nil, fmt.Errorf("go tool pprof -raw err: %v", err)
	}

	profile, err := torchPprof.ParseRaw(out)
	if err != nil {
		return nil, fmt.Errorf("could not parse raw pprof output: %v", err)
	}

	sampleIndex := torchPprof.SelectSample([]string{}, profile.SampleNames)
	flameInput, err := renderer.ToFlameInput(profile, sampleIndex)
	if err != nil {
		return nil, fmt.Errorf("could not convert stacks to flamegraph input: %v", err)
	}
	if len(flameInput) == 0 {
		return []byte{}, nil
	}

	data, err = renderer.GenerateFlameGraph(flameInput)
	if err != nil {
		elog.Error("flame graph err", zap.Error(err), zap.Any("flameInput", flameInput))
		return nil, fmt.Errorf("could not generate flame graph: %v", err)
	}

	return
}

func (p *pprof) generateProfileSvg(rawFilePath, svgFilePath string) (data []byte, err error) {
	_, err = exec.Command("bash", "-c", fmt.Sprintf("go tool pprof -svg %s > %s", rawFilePath, svgFilePath)).Output()
	if err != nil {
		return nil, fmt.Errorf("profile svg 生成失败: %v", err)
	}

	data, err = ioutil.ReadFile(svgFilePath)
	if err != nil {
		return nil, errors.Wrap(err, "读取Profile SVG文件失败")
	}

	return
}

func getPprofUrl(profileType, UniqueKey, svgType string) string {
	return fmt.Sprintf(econf.GetString("app.rootURL")+"/graph?goType=%s&url=%s&svgType=%s", profileType, UniqueKey, svgType)
}
