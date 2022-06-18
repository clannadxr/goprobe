package dto

type (
	// ReqRunProfile ..
	ReqRunProfile struct {
		Mode        string `form:"mode" json:"mode" binding:"required"` // Pod, Ip
		ClusterName string `form:"clusterName"`
		PodName     string `form:"podName"`
		Port        int    `form:"port"`
		Namespace   string `form:"namespace"`
		Addr        string `form:"addr" json:"addr"`
		Seconds     int    `form:"seconds" json:"seconds"`
		Type        int    `form:"type"`
		Token       string `form:"token"`
		TempFileDir string
		UniqueKey   string
	}

	ReqPprofGraph struct {
		SvgType string `form:"svgType"` // flame | profile
		GoType  string `form:"goType"`  // block | goroutine | heap | profile
		Url     string `form:"url"`
	}
)
