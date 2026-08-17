# BENZHI_README

## 项目说明
- 项目：benzhi-project-8a5d9c16-cd8b-48dd-984b-dbab6a678b68
- 项目用途：Implemented BagHold HTTP JSON service with mutex-protected state, strict decoding, typed errors, defensive snapshots, cancellation-safe mutations, immutable assessments, tests, documentation, and deterministic smoke mode.
- Go 工具链：`golang:1.22.0`
- 前端工具链：无

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/baghold smoke
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-8a5d9c16-cd8b-48dd-984b-dbab6a678b68-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-8a5d9c16-cd8b-48dd-984b-dbab6a678b68-arm64 linux/arm64
docker run -it benzhi-project-8a5d9c16-cd8b-48dd-984b-dbab6a678b68-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/baghold smoke`
