/*
Copyright (c) 2026 a05777. All rights reserved. This software is provided "as is", without warranty of any kind. See LICENSE for details.
软件许可及免责声明
版权所有 (c) 2026 a05777。保留所有权利。

1. 权利保留与授权范围 (Rights & Scope)
本软件的所有权、知识产权及源代码权利均归原作者所有。获得源代码的人员（“用户”）仅拥有非营利性的个人学习、研究及调试代码的非排他性权利。未经原作者明确书面许可，严禁将本软件或其任何部分用于商业盈利、集成至商业产品、或通过网络对外提供付费/免费服务（如 SaaS/API）。

2. 绝对免责声明 (No Warranty)
本程序按“原样”（"AS IS"）提供，不附带任何形式的明示或暗示保证。作者不保证程序符合特定用途，亦不保证运行过程中不出现错误。

3. 责任限制 (Limitation of Liability)
在任何情况下，作者不对因使用本程序产生的任何损害（包括数据丢失、系统崩溃、法律诉讼等）承担任何责任。作者的全部赔偿责任上限在任何情况下均不超过用户实际支付的授权费用（如有）。

4. 风险告知与解释权 (Acceptance & Interpretation)
用户一旦运行、调试或以任何方式使用本程序，即视为完全理解并接受上述条款。作者保留对本协议的最终解释权，并有权随时更新授权条款。

*/


package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	EnableSSL bool   `json:"enable_ssl"`
	HTTPSPort string `json:"https_port"`
	HTTPPort  string `json:"http_port"`
	Domain    string `json:"domain"`
}

var indexCache []byte

// sniffingListener 核心：在 TCP 握手阶段探测协议
type sniffingListener struct {
	net.Listener
	tlsConfig *tls.Config
	domain    string
	port      string
}

func (l *sniffingListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}

		br := bufio.NewReader(c)
		peek, err := br.Peek(1)
		if err != nil {
			c.Close()
			continue
		}

		if peek[0] == 0x16 {
			return tls.Server(&bufferedConn{Conn: c, br: br}, l.tlsConfig), nil
		}

		target := fmt.Sprintf("https://%s:%s", l.domain, l.port)
		redirectMsg := fmt.Sprintf("HTTP/1.1 301 Moved Permanently\r\n"+
			"Location: %s\r\n"+
			"Content-Length: 0\r\n"+
			"Connection: close\r\n\r\n", target)

		c.Write([]byte(redirectMsg))
		c.Close()
		continue
	}
}

type bufferedConn struct {
	net.Conn
	br *bufio.Reader
}

func (bc *bufferedConn) Read(b []byte) (int, error) { return bc.br.Read(b) }

type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}

func main() {
	// 1. 读取并解析配置
	configData, err := os.ReadFile("config.json")
	if err != nil {
		log.Printf("Warning: 找不到 config.json，将使用默认配置或尝试启动...")
		// 如果必须依赖配置，这里可以保留 Fatal，或者设置默认值
	}
	var cfg Config
	_ = json.Unmarshal(configData, &cfg) // 即使解析失败也尝试继续

	// 2. 尝试预加载 index.html (改为非阻塞)
	indexPath := filepath.Join("html", "index.html")
	indexCache, err = os.ReadFile(indexPath)
	if err != nil {
		log.Printf("Warning: 未能预加载 %s (%v)。服务器将正常启动，但首页可能无法显示。", indexPath, err)
	} else {
		log.Printf("Success: 已成功预加载首页内容。")
	}

	// 3. 统一路由处理器
	mux := http.NewServeMux()

	// 静态资源服务，根目录设为 html
	fileServer := http.FileServer(http.Dir("html"))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 资源限制
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

		// 安全防护
		cleanPath := strings.ToLower(r.URL.Path)
		if strings.Contains(cleanPath, "/ssl") || strings.Contains(cleanPath, "config.json") {
			http.Error(w, "ACCESS_DENIED", http.StatusForbidden)
			return
		}

		// 优先处理首页
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			if len(indexCache) > 0 {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(indexCache)
				return
			}
			// 如果内存没缓存，尝试实时读一次磁盘（容错）
			data, err := os.ReadFile(filepath.Join("html", "index.html"))
			if err == nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(data)
				return
			}
			http.Error(w, "Index file not found", http.StatusNotFound)
			return
		}

		// 检查 html 目录下是否存在该文件
		targetFile := filepath.Join("html", filepath.Clean(r.URL.Path))
		info, err := os.Stat(targetFile)

		if err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		http.NotFound(w, r)
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

// 4. 运行逻辑
    if !cfg.EnableSSL {
        if cfg.HTTPPort == "" { cfg.HTTPPort = "80" }
        log.Printf("[HTTP] 监听端口: %s", cfg.HTTPPort)
        log.Fatal(srv.ListenAndServe())
    } else {
        cert, err := tls.LoadX509KeyPair(filepath.Join("ssl", "server.crt"), filepath.Join("ssl", "server.key"))
        if err != nil {
            log.Printf("Fatal: 开启了 SSL 但找不到证书文件。请检查 ssl/ 目录。")
            log.Fatal(err)
        }

        // 核心修改：配置 TLS 协议协商 (ALPN)
        tlsConfig := &tls.Config{
            Certificates: []tls.Certificate{cert},
            MinVersion:   tls.VersionTLS12,
            // NextProtos 是开启 HTTP/2 的关键
            // 浏览器会根据这个列表与服务器协商，优先使用 h2
            NextProtos:   []string{"h2", "http/1.1"}, 
        }

        // 必须将 tlsConfig 关联到 srv，否则 srv 内部无法初始化 HTTP/2 控制器
        srv.TLSConfig = tlsConfig

        ln, err := net.Listen("tcp", ":"+cfg.HTTPSPort)
        if err != nil {
            log.Fatal(err)
        }

        if tcpLn, ok := ln.(*net.TCPListener); ok {
            ln = tcpKeepAliveListener{tcpLn}
        }

        // 提示信息优化：增加 H2 状态显示
        log.Printf("[HTTPS 混合模式] 监听端口: %s, 域名: %s (支持 HTTP/2)", cfg.HTTPSPort, cfg.Domain)

        l := &sniffingListener{
            Listener:  ln,
            tlsConfig: tlsConfig,
            domain:    cfg.Domain,
            port:      cfg.HTTPSPort,
        }

        // 由于 srv.TLSConfig 已设置，srv.Serve(l) 会自动识别并升级协议
        log.Fatal(srv.Serve(l))
    }
}