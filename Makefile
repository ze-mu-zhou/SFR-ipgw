NAME=ipgw
REPO=ze-mu-zhou/SFR-ipgw
MAIN_ENTRY=cmd/ipgw/main.go
VERSION=$(shell git describe --tags --exact-match 2>/dev/null || echo dev)
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DIRTY=$(shell test -z "$$(git status --porcelain)" || echo .dirty)
BUILD_KIND=local
UPDATE_ENABLED=false
RELEASE_REPO=
BUILD_VERSION=$(VERSION)+local.$(COMMIT)$(DIRTY)
BUILD=$(shell date +%FT%T%z)
BUILD_DIR=build
RELEASE_DIR=release
GO_BUILD=CGO_ENABLED=0 go build -trimpath -ldflags '-w -s -X "github.com/ze-mu-zhou/SFR-ipgw.Version=${BUILD_VERSION}" \
		-X "github.com/ze-mu-zhou/SFR-ipgw.Build=${BUILD}" -X "github.com/ze-mu-zhou/SFR-ipgw.Repo=${REPO}" -X "github.com/ze-mu-zhou/SFR-ipgw.Commit=${COMMIT}" -X "github.com/ze-mu-zhou/SFR-ipgw.BuildKind=${BUILD_KIND}" -X "github.com/ze-mu-zhou/SFR-ipgw.ReleaseRepo=${RELEASE_REPO}" -X "github.com/ze-mu-zhou/SFR-ipgw.UpdateEnabled=${UPDATE_ENABLED}"'

.PHONY: clean

PLATFORM_LIST = \
	darwin-amd64 \
	darwin-arm64 \
	linux-386 \
	linux-amd64 \
	linux-arm \
	linux-mips64 \
	linux-mips64le \
	freebsd-386 \
	freebsd-amd64 \
	windows-386 \
	windows-amd64 \
	windows-arm64

all: clean $(PLATFORM_LIST)

darwin-amd64:
	GOARCH=amd64 GOOS=darwin $(GO_BUILD) -o $(BUILD_DIR)/$@/$(NAME) ${MAIN_ENTRY}

darwin-arm64:
	GOARCH=arm64 GOOS=darwin $(GO_BUILD) -o $(BUILD_DIR)/$@/$(NAME) ${MAIN_ENTRY}

linux-386:
	GOARCH=386 GOOS=linux $(GO_BUILD) -o $(BUILD_DIR)/$@/$(NAME) ${MAIN_ENTRY}

linux-amd64:
	GOARCH=amd64 GOOS=linux $(GO_BUILD) -o $(BUILD_DIR)/$@/$(NAME) ${MAIN_ENTRY}

linux-arm:
	GOARCH=arm GOOS=linux $(GO_BUILD) -o $(BUILD_DIR)/$@/$(NAME) ${MAIN_ENTRY}

linux-mips64:
	GOARCH=mips64 GOOS=linux $(GO_BUILD) -o $(BUILD_DIR)/$@/$(NAME) ${MAIN_ENTRY}

linux-mips64le:
	GOARCH=mips64le GOOS=linux $(GO_BUILD) -o $(BUILD_DIR)/$@/$(NAME) ${MAIN_ENTRY}

freebsd-386:
	GOARCH=386 GOOS=freebsd $(GO_BUILD) -o $(BUILD_DIR)/$@/$(NAME) ${MAIN_ENTRY}

freebsd-amd64:
	GOARCH=amd64 GOOS=freebsd $(GO_BUILD) -o $(BUILD_DIR)/$@/$(NAME) ${MAIN_ENTRY}

windows-386:
	GOARCH=386 GOOS=windows $(GO_BUILD) -o $(BUILD_DIR)/$@/$(NAME).exe ${MAIN_ENTRY}

windows-amd64:
	GOARCH=amd64 GOOS=windows $(GO_BUILD) -o $(BUILD_DIR)/$@/$(NAME).exe ${MAIN_ENTRY}

windows-arm64:
	GOARCH=arm64 GOOS=windows $(GO_BUILD) -o $(BUILD_DIR)/$@/$(NAME).exe ${MAIN_ENTRY}

release: all
	bash scripts/release.sh $(NAME) $(BUILD_DIR) $(RELEASE_DIR)

clean:
	rm -rf $(BUILD_DIR)/*
