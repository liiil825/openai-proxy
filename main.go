package main

import (
  "context"
  "crypto/tls"
  "io"
  "log"
  "net"
  "net/http"
  "net/url"
  "os"
  "strings"
  "time"
)

var (
  target    = "https://api.openai.com" // 目标域名
  httpProxy = "http://127.0.0.1:10809" // 本地代理地址和端口
)

func main() {
  http.HandleFunc("/", handleRequest)
  log.Fatal(http.ListenAndServe(":9000", nil))
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
  // 过滤无效 URL
  _, err := url.ParseRequestURI(r.RequestURI)
  if err != nil {
    log.Println("Error parsing URL: ", err.Error())
    http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
    return
  }

  // 去掉环境前缀（针对腾讯云，如果包含的话，目前我只用到了test和release）
  newPath := strings.Replace(r.URL.Path, "/release", "", 1)
  newPath = strings.Replace(newPath, "/test", "", 1)

  // 拼接目标 URL
  targetURL := target + newPath

  // 创建代理 HTTP 请求
  proxyReq, err := http.NewRequestWithContext(context.Background(), r.Method, targetURL, r.Body)
  if err != nil {
    log.Println("Error creating proxy request: ", err.Error())
    http.Error(w, "Error creating proxy request", http.StatusInternalServerError)
    return
  }

  // 复制原始请求头到新请求中
  proxyReq.Header = r.Header.Clone()

  // 复制 POST 请求体到新请求中
  if r.Method == http.MethodPost || r.Method == http.MethodPut {
    proxyReq.Body = r.Body
  }

  // 设置超时时间和连接池
  transport := &http.Transport{
    DialContext: (&net.Dialer{
      Timeout:   30 * time.Second,
      KeepAlive: 30 * time.Second,
    }).DialContext,
    TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
  }
  client := &http.Client{
    Transport: transport,
    Timeout:   60 * time.Second,
  }

  // 本地测试通过代理请求 OpenAI 接口
  if os.Getenv("ENV") == "local" {
    proxyURL, _ := url.Parse(httpProxy) // 本地HTTP代理配置
    transport.Proxy = http.ProxyURL(proxyURL)
  }

  // 向 OpenAI 发起代理请求
  resp, err := client.Do(proxyReq)
  if err != nil {
    log.Println("Error sending proxy request: ", err.Error())
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
  defer resp.Body.Close()

  // 复制响应头到代理响应头中
  w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
  w.Header().Set("Cache-Control", resp.Header.Get("Cache-Control"))
  w.Header().Set("Expires", resp.Header.Get("Expires"))
  w.Header().Set("Last-Modified", resp.Header.Get("Last-Modified"))
  w.Header().Set("ETag", resp.Header.Get("ETag"))

  // 设置响应状态码
  w.WriteHeader(resp.StatusCode)

  // 将响应实体写入到响应流中（支持流式响应）
  io.Copy(w, resp.Body)
}

