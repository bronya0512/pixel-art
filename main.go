// Package main 实现一个简单的图片转像素画 Web 服务。
//
// 前端：static/index.html（HTML + JS）
// 后端：接收图片上传，调用 pixelate.py 生成像素画并返回。
//
// 运行：
//
//	go run .
//
// 然后浏览器打开 http://localhost:8080
package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed static
var staticFS embed.FS

const (
	listenAddr = ":8080"
	scriptName = "pixelate.py"
	maxUpload  = 20 << 20 // 20MB
	outputDir  = "output"  // 像素画持久化保存目录

	chatAPIURL = "https://tokenhub.tencentmaas.com/v1/chat/completions"
	chatModel  = "hy3"
)

// pythonCandidates 按顺序尝试可用的 Python 解释器。
var pythonCandidates = []string{"python", "python3", "py"}

func main() {
	// 创建像素画持久化保存目录
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Fatalf("创建输出目录失败: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/chat", handleChatPage)
	mux.HandleFunc("/api/pixelate", handlePixelate)
	mux.HandleFunc("/api/pixelate-json", handlePixelateJSON)
	mux.HandleFunc("/api/chat", handleChatAPI)
	// 静态文件服务：/files/xxx.png 访问 outputDir 下的文件
	mux.Handle("/files/", http.StripPrefix("/files/", http.FileServer(http.Dir(outputDir))))

	log.Printf("像素画服务已启动: http://localhost%s", listenAddr)
	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

// handleIndex 返回前端页面。
func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "无法加载页面", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// handleChatPage 返回聊天页面。
func handleChatPage(w http.ResponseWriter, r *http.Request) {
	data, err := staticFS.ReadFile("static/chat.html")
	if err != nil {
		http.Error(w, "无法加载聊天页面", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// chatMessage 是 OpenAI 兼容接口的消息结构。
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatAPIKey 从环境变量读取，避免把密钥硬编码进代码仓库。
func chatAPIKey() string {
	return strings.TrimSpace(os.Getenv("CHAT_API_KEY"))
}

// handleChatAPI 将聊天请求转发到腾讯云大模型，并以 SSE 流式返回。
func handleChatAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST", http.StatusMethodNotAllowed)
		return
	}

	key := chatAPIKey()
	if key == "" {
		http.Error(w, "服务端未配置 CHAT_API_KEY 环境变量", http.StatusInternalServerError)
		return
	}

	var req struct {
		Messages []chatMessage `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "请求解析失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Messages) == 0 {
		http.Error(w, "messages 不能为空", http.StatusBadRequest)
		return
	}

	payload := map[string]any{
		"model":      chatModel,
		"messages":   req.Messages,
		"stream":     true,
		"max_tokens": 1024,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "请求构造失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	upstream, err := http.NewRequest(http.MethodPost, chatAPIURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "创建上游请求失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	upstream.Header.Set("Content-Type", "application/json")
	upstream.Header.Set("Authorization", "Bearer "+key)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(upstream)
	if err != nil {
		http.Error(w, "调用大模型失败: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		http.Error(w, fmt.Sprintf("上游返回 %d: %s", resp.StatusCode, string(msg)), http.StatusBadGateway)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "不支持流式响应", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
			return
		}
		flusher.Flush()
	}
	if err := scanner.Err(); err != nil {
		log.Printf("读取上游流失败: %v", err)
	}
}

// jsonResponse 是 JSON 接口的统一返回结构。
type jsonResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	URL  string `json:"url,omitempty"`
}

// handlePixelate 处理图片上传与转换请求（返回二进制图片，供 Web 端使用）。
func handlePixelate(w http.ResponseWriter, r *http.Request) {
	outData, err := doPixelate(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Disposition", `attachment; filename="pixel_art.png"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(outData)))
	w.Write(outData)
}

// handlePixelateJSON 处理图片上传并返回 JSON（供微信小程序使用）。
// 生成的像素画会保存到 outputDir，通过 /files/ 静态访问。
func handlePixelateJSON(w http.ResponseWriter, r *http.Request) {
	outData, err := doPixelate(w, r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, jsonResponse{Code: 1, Msg: err.Error()})
		return
	}

	filename := randomHex(8) + ".png"
	if err := os.WriteFile(filepath.Join(outputDir, filename), outData, 0o644); err != nil {
		writeJSON(w, http.StatusInternalServerError, jsonResponse{Code: 1, Msg: "保存文件失败: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, jsonResponse{Code: 0, Msg: "ok", URL: "/files/" + filename})
}

// doPixelate 解析上传、调用脚本并返回生成的 PNG 字节。
func doPixelate(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if r.Method != http.MethodPost {
		return nil, fmt.Errorf("仅支持 POST")
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		return nil, fmt.Errorf("表单解析失败或文件过大: %v", err)
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		return nil, fmt.Errorf("缺少图片字段 image: %v", err)
	}
	defer file.Close()

	// 解析参数（限制为整数，避免命令注入）
	size := parseIntArg(r, "size", 12, 1, 200)
	colors := parseIntArg(r, "colors", 0, 0, 256)
	widthBlocks := parseIntArg(r, "width_blocks", 0, 0, 512)

	workDir, err := os.MkdirTemp("", "pixelate-*")
	if err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(workDir)

	inputPath := filepath.Join(workDir, sanitizeName(header.Filename))
	outputPath := filepath.Join(workDir, "output.png")

	if err := saveUpload(file, inputPath); err != nil {
		return nil, fmt.Errorf("保存上传文件失败: %v", err)
	}
	if err := runScript(inputPath, outputPath, size, colors, widthBlocks); err != nil {
		return nil, fmt.Errorf("像素画生成失败: %v", err)
	}

	outData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("读取结果失败: %v", err)
	}
	return outData, nil
}

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// randomHex 生成 n 字节的随机十六进制字符串。
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}

// runScript 调用 pixelate.py 生成像素画。
func runScript(inputPath, outputPath string, size, colors, widthBlocks int) error {
	python, err := findPython()
	if err != nil {
		return err
	}

	script := scriptName
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("找不到脚本 %s，请确认与程序在同一目录: %w", scriptName, err)
	}

	args := []string{script, inputPath, "-o", outputPath, "-s", strconv.Itoa(size)}
	if colors > 0 {
		args = append(args, "-c", strconv.Itoa(colors))
	}
	if widthBlocks > 0 {
		args = append(args, "-w", strconv.Itoa(widthBlocks))
	}

	cmd := exec.Command(python, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("执行 %s 失败: %v; %s", python, err, stderr.String())
	}

	if _, err := os.Stat(outputPath); err != nil {
		return fmt.Errorf("脚本未生成输出文件: %w", err)
	}
	return nil
}

// findPython 返回第一个可用的 Python 解释器。
func findPython() (string, error) {
	for _, name := range pythonCandidates {
		if _, err := exec.LookPath(name); err == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("未找到 Python，请先安装 Python 并执行 pip install pillow")
}

// parseIntArg 解析表单中的整数参数，带默认值与范围限制。
func parseIntArg(r *http.Request, key string, def, minV, maxV int) int {
	raw := strings.TrimSpace(r.FormValue(key))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

// sanitizeName 只保留文件名部分，去除路径。
func sanitizeName(name string) string {
	name = filepath.Base(name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "upload_" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".png"
	}
	return name
}

// saveUpload 将上传文件写入磁盘。
func saveUpload(file multipart.File, dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, file)
	return err
}
