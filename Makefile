TARGET = budgetbridge

SRC := $(wildcard *.go)

all: $(TARGET)

$(TARGET): build

lint:
	go run honnef.co/go/tools/cmd/staticcheck

test:
	go test -v

build: lint test
	go build
