#!/bin/sh

go mod tidy
CGO_ENABLED=0 go build -o aajonus.online -ldflags '-s -w -extldflags "-static"' -trimpath .
