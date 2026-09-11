.DEFAULT_GOAL := help
BIN := bin/egov-law-sync
PKG := ./cmd/egov-law-sync
COVER_MIN := 80
CRAP_MAX := 15
GOCYCLO := github.com/fzipp/gocyclo/cmd/gocyclo@v0.6.0
GREMLINS := github.com/go-gremlins/gremlins/cmd/gremlins@v0.5.0
UNIT_PKGS = $$(go list ./... | grep -vE '/cmd/|/e2e')

.PHONY: help build test shelltest crap mutate e2e lint doclint clean

help: ## ターゲット一覧を表示する
	@awk 'BEGIN{FS=":.*## "} /^[a-zA-Z_-]+:.*## /{printf "  %-10s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## CGOなしで静的バイナリをbin/に作る
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN) $(PKG)

test: shelltest ## 単体テストを実行し、合計カバレッジが80%未満なら失敗する（cmd/とe2e/は対象外）
	@mkdir -p bin
	go test -race -coverprofile=bin/cover.out -coverpkg=$$(go list ./... | grep -vE '/cmd/|/e2e' | paste -sd, -) $(UNIT_PKGS)
	@total=$$(go tool cover -func=bin/cover.out | awk '/^total:/{sub("%","",$$3); print $$3}'); \
	echo "coverage: $$total%"; \
	awk -v t="$$total" -v m=$(COVER_MIN) 'BEGIN{ if (t+0 < m) { print "coverage " t "% is below " m "%"; exit 1 } }'

shelltest: ## tools/ のshellスクリプトを検査する
	bash tools/next-version_test.sh

crap: test ## 全関数のCRAP値を計算し、15を超える関数があれば失敗する
	@go run $(GOCYCLO) -over 0 . > bin/cyclo.txt
	@go tool cover -func=bin/cover.out > bin/coverfunc.txt
	@bash tools/crap.sh bin/cyclo.txt bin/coverfunc.txt $(CRAP_MAX)

mutate: ## internal/domain 配下にmutation testingを実行し、生存mutantがあれば失敗する
	go run $(GREMLINS) unleash ./internal/domain --workers 1 --timeout-coefficient 10 --threshold-efficacy 100 --threshold-mcover 90

e2e: build ## ビルドしたバイナリで主要導線のE2Eテストを実行する
	EGOV_BIN=$(CURDIR)/$(BIN) go test -count=1 ./e2e/...

lint: ## go vet と gofmt の差分確認、層の依存検査
	go vet ./...
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "gofmt: 上のファイルを整形してください"; exit 1; }
	bash tools/layers.sh

doclint: ## docs/ の設計文書と Go のコメント行を機械検査する
	bash tools/doclint.sh

clean: ## bin/ を削除する
	rm -rf bin
