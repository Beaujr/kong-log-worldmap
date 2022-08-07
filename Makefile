GOARCH ?= arm64
GOOS ?= linux
build:
	@GOARCH=$(GOARCH) GOOS=$(GOOS) go build -o logwatcher

