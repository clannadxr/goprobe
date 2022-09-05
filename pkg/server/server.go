package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/econf"
	"github.com/gotomicro/ego/core/elog"
	"github.com/gotomicro/ego/core/etrace"
	"github.com/gotomicro/ego/server/egin"

	"goprobe/pkg/dto"
	"goprobe/pkg/pprof"
)

func ServeHTTP() *egin.Component {
	router := egin.Load("server.http").Build()
	router.GET("/api/pprof/run", func(ctx *gin.Context) {
		var params dto.ReqRunProfile
		err := ctx.Bind(&params)
		if err != nil {
			JSONE(ctx, 1, "参数无效: "+err.Error(), nil)
			return
		}
		if params.Token != econf.GetString("token") {
			JSONE(ctx, 1, "Token无效 ", nil)
			return
		}
		list, err := pprof.Pprof.GeneratePprof(params)
		if err != nil {
			JSONE(ctx, 1, "生成pprof: "+err.Error(), nil)
			return
		}
		JSONOK(ctx, list)
	})
	router.GET("/graph", Graph)
	router.GET("/pprof-list", GetPprofList)
	return router
}

func Graph(c *gin.Context) {
	var params dto.ReqPprofGraph
	err := c.Bind(&params)
	if err != nil {
		JSONE(c, 1, "参数无效: "+err.Error(), nil)
		return
	}

	data, err := pprof.Pprof.FindGraphData(params)
	if err != nil {
		JSONE(c, 1, "FindGraphData: "+err.Error(), nil)
		return
	}
	c.Data(http.StatusOK, "image/svg+xml", data)
}

func GetPprofList(c *gin.Context) {
	var params dto.ReqGetPprofList
	err := c.Bind(&params)
	if err != nil {
		JSONE(c, 1, "参数无效: "+err.Error(), nil)
		return
	}
	data, err := pprof.Pprof.GetPprofList(params)
	if err != nil {
		JSONE(c, 1, "GetPprofList: "+err.Error(), nil)
		return
	}
	JSONOK(c, data)
}

// JSONE 输出失败响应
// 形如 {"code":<code>, "msg":<msg>, "data":<data>}
func JSONE(c *gin.Context, code int, msg string, data interface{}) {
	j := new(Res)
	j.Code = code
	j.Msg = msg
	switch d := data.(type) {
	case error:
		j.Data = d.Error()
	default:
		j.Data = data
	}
	elog.Warn("biz warning", elog.FieldValue(msg), elog.FieldValueAny(data), elog.FieldTid(etrace.ExtractTraceID(c.Request.Context())))
	c.JSON(http.StatusOK, j)
	return
}

// JSONOK 输出响应成功JSON，如果data不为零值，则输出data
// 形如 {"code":0, "msg":"成功", "data":<data>}
func JSONOK(c *gin.Context, data any) {
	j := new(Res)
	j.Code = 0
	j.Msg = "成功"
	j.Data = data
	c.JSON(http.StatusOK, j)
	return
}

// Res 标准JSON输出格式
type Res struct {
	// Code 响应的业务错误码。0表示业务执行成功，非0表示业务执行失败。
	Code int `json:"code"`
	// Msg 响应的参考消息。前端可使用msg来做提示
	Msg string `json:"msg"`
	// Data 响应的具体数据
	Data interface{} `json:"data"`
}
