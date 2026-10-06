# Sample Makefile for query context tests
SHELL := /bin/bash
GO ::= go
VERSION ?= dev
LDFLAGS = -X main.version=$(VERSION)

.PHONY: build test lint clean
.DEFAULT_GOAL := build

build: deps
	$(GO) build ./...

test lint: build
	$(GO) test ./...

%.o: %.c
	cc -c $<

$(BIN): main.go
	go build -o $@

bin/tool: tool.go
	go build -o $@

clean::
	rm -rf bin

deps:
release: export GOFLAGS = -trimpath
release: build
