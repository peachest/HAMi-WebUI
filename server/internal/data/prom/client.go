package prom

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type Client struct {
	client  api.Client
	timeout time.Duration
}

type CustomTransport struct {
	auth      string
	enableLog bool
	http.RoundTripper
}

// RequestLog 记录请求和响应的结构体
type RequestLog struct {
	Timestamp      int64         `json:"timestamp"`
	Method         string        `json:"method"`
	URL            string        `json:"url"`
	QueryParams    string        `json:"query_params"`
	RequestBody    string        `json:"request_body"`
	RequestHeaders http.Header   `json:"request_headers"`
	ResponseCode   int           `json:"response_code"`
	ResponseBody   string        `json:"response_body"`
	Duration       time.Duration `json:"duration"`
}

func (c *CustomTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var err error
	start := time.Now() // 记录开始时间
	defer func() {
		if err != nil {
			log.Infof("Error in RoundTrip, start time: %v, cost: %v\n", start, time.Since(start))
		}
	}()

	req.Header.Set("Authorization", c.auth)
	if !c.enableLog {
		return c.RoundTripper.RoundTrip(req)
	}

	var requestBody []byte
	var responseBody []byte
	var log RequestLog

	// 只有在需要记录日志时才读取请求体和响应体
	// 读取请求体
	if req.Body != nil {
		requestBody, err = ioutil.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		// 重新设置请求体，以便后续的处理
		req.Body = ioutil.NopCloser(bytes.NewReader(requestBody))
	}

	// 调用原始的 RoundTripper
	resp, err := c.RoundTripper.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 计算耗时
	duration := time.Since(start) / time.Millisecond

	// 读取响应体
	responseBody, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 创建请求日志
	log = RequestLog{
		Timestamp:      start.UnixNano() / int64(time.Millisecond),
		Method:         req.Method,
		URL:            req.URL.String(),
		QueryParams:    req.URL.RawQuery,
		RequestBody:    string(requestBody),
		RequestHeaders: req.Header,
		ResponseCode:   resp.StatusCode,
		ResponseBody:   string(responseBody),
		Duration:       duration,
	}

	// 记录到 JSONL 文件
	file, err := os.OpenFile("./requests.jsonl", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	logLine, err := json.Marshal(log)
	if err != nil {
		return nil, err
	}
	if _, err := file.Write(append(logLine, '\n')); err != nil {
		return nil, err
	}

	// 重新设置响应体，以便后续的处理
	resp.Body = ioutil.NopCloser(bytes.NewReader(responseBody))

	fmt.Printf("Response Status: %s, Duration: %dms\n", resp.Status, duration)
	return resp, nil
}

// func (t *CustomTransport) RoundTrip(req *http.Request) (*http.Response, error) {
// 	req.Header.Set("Authorization", t.auth)
// 	// log.Infof("Request Method: %s, URL: %s \nBody: %s", req.Method, req.URL, req.Body)

// 	fmt.Println("=== 请求详情 ===")
// 	fmt.Printf("Method: %s\nURL: %s\n", req.Method, req.URL)

// 	// 解析并打印查询参数
// 	fmt.Println("\n查询参数:")
// 	for k, v := range req.URL.Query() {
// 		fmt.Printf("  %-15s: %v\n", k, v)
// 	}

// 	// 打印请求头
// 	fmt.Println("\n请求头:")
// 	for k, v := range req.Header {
// 		fmt.Printf("  %-15s: %v\n", k, v)
// 	}

// 	// 读取并打印请求体
// 	var bodyBytes []byte
// 	if req.Body != nil {
// 		var err error
// 		bodyBytes, err = io.ReadAll(req.Body)
// 		if err != nil {
// 			return nil, fmt.Errorf("读取请求体失败: %v", err)
// 		}
// 		// 恢复Body以便后续处理
// 		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
// 	}
// 	fmt.Printf("\n请求体:\n%s\n", string(bodyBytes))
// 	fmt.Println("===================")

// 	return t.Transport.RoundTrip(req)
// }

func NewClient(address string, timeout time.Duration, auth string) (*Client, error) {
	enableLog := os.Getenv("ENABLE_PROM_LOG_FILE") == "1"
	client, err := api.NewClient(api.Config{
		Address: address,
		RoundTripper: &CustomTransport{
			auth:      auth,
			enableLog: enableLog,
			RoundTripper: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, // 忽略 SSL 证书验证
				},
			},
		},
	})
	if err != nil {
		fmt.Printf("Error creating client: %v\n", err)
		return nil, fmt.Errorf("error creating client: %v", err)
	}
	return &Client{
		client,
		timeout,
	}, nil
}

func (c *Client) Conn() (api.Client, error) {
	return c.client, nil
}

// Query 查询单点时刻指标值
func (c *Client) Query(ctx context.Context, query string) (model.Value, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	v1api := v1.NewAPI(c.client)
	result, warnings, err := v1api.Query(ctx, query, time.Now(), v1.WithTimeout(c.timeout))
	if err != nil {
		log.Errorf("Error querying Prometheus, query: %v, err: %v\n", query, err)
		return result, fmt.Errorf("error querying Prometheus: %v", err)
	}
	if len(warnings) > 0 {
		log.Warnf("Warnings: %v\n", warnings)
		return result, fmt.Errorf("warnings: %v", warnings)
	}
	return result, nil
}

// QueryRange 查询时间范围内指标变化趋势数据
func (c *Client) QueryRange(ctx context.Context, query string, r v1.Range) (model.Value, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	v1api := v1.NewAPI(c.client)
	result, warnings, err := v1api.QueryRange(ctx, query, r, v1.WithTimeout(c.timeout))
	if err != nil {
		log.Errorf("Error querying Prometheus, query: %v, err: %v\n", query, err)
		return result, fmt.Errorf("error querying Prometheus: %v", err)
	}
	if len(warnings) > 0 {
		log.Warnf("Warnings: %v\n", warnings)
		return result, fmt.Errorf("warnings: %v", warnings)
	}
	return result, nil
}
