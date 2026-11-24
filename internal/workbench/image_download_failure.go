package workbench

import (
	"regexp"
	"strings"

	"github.com/sylvinus/agent-vm/internal/vm"
)

// The summary line carries only fixed codes; anything else is not accepted,
// so process output never becomes an arbitrary user-facing message.
var imageDownloadLine = regexp.MustCompile(`^` + vm.ImageDownloadMarker + `((?: (?:official|nju|ustc|other):(?:http-[1-5][0-9][0-9]|eof|timeout|tls|dns|refused|checksum|other):(?:kept|none))+)\s*$`)

var imageMirrorNames = map[string]string{
	"official": "Debian 官方源",
	"nju":      "南京大学镜像",
	"ustc":     "中国科学技术大学镜像",
	"other":    "其他下载地址",
}

var imageCauseNames = map[string]string{
	"http-403": "HTTP 403，服务器拒绝访问",
	"http-404": "HTTP 404，文件不存在",
	"http-429": "HTTP 429，请求过于频繁",
	"eof":      "连接被中途断开",
	"timeout":  "连接超时",
	"tls":      "TLS 或证书错误",
	"dns":      "域名解析失败",
	"refused":  "连接被拒绝",
	"checksum": "文件校验不符，下载内容已丢弃",
	"other":    "其他错误",
}

// imageDownloadFailure explains a failed Debian image download by location:
// which mirror, why, and whether partial data remains for resuming.
func imageDownloadFailure(output string) (reason, next string, ok bool) {
	var tokens []string
	for _, line := range strings.Split(output, "\n") {
		if m := imageDownloadLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			tokens = strings.Fields(m[1])
		}
	}
	if len(tokens) == 0 {
		return "", "", false
	}
	parts, kept := make([]string, 0, len(tokens)), false
	for _, token := range tokens {
		fields := strings.Split(token, ":")
		cause, known := imageCauseNames[fields[1]]
		if !known {
			cause = "HTTP " + strings.TrimPrefix(fields[1], "http-")
			if strings.HasPrefix(fields[1], "http-5") {
				cause += "，服务器错误"
			}
		}
		if fields[2] == "kept" {
			kept = true
			cause += "，已保留下载进度"
		}
		parts = append(parts, imageMirrorNames[fields[0]]+"（"+cause+"）")
	}
	reason = "Debian 镜像的下载地址都未能完成下载：" + strings.Join(parts, "；")
	next = "镜像由主机下载。请检查主机网络及代理设置，确认能访问镜像站后继续安装"
	if kept {
		next += "；已下载的部分会保留，继续安装时从断点续传"
	}
	return reason, next + "。", true
}
