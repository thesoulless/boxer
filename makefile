TEST?=./...
GOFMT_FILES?=$$(find . -not -path "./vendor/*" -type f -name '*.go')

default: test

# test runs the unit tests
# we run this one package at a time here because running the entire suite in
# one command creates memory usage issues when running in Travis-CI.
test: fmtcheck
	go list $(TEST) | xargs -t -n4 go test -v $(TESTARGS) -timeout=2m -parallel=4 -count=1

# testrace runs the race checker
testrace: fmtcheck
	go test -race $(TEST) $(TESTARGS)

coverage:
	@go tool cover 2>/dev/null; if [ $$? -eq 3 ]; then \
		go get -u golang.org/x/tools/cmd/cover; \
	fi
	go test $(TEST) -coverprofile=coverage.out
	go tool cover -html=coverage.out
	rm coverage.out

fmtcheck:
	@echo "==> Checking that code complies with gofmt requirements..."
	go fmt ./...

docs:
	@godoc -http=:6060

.PHONY: coverage fmtcheck test testrace docs