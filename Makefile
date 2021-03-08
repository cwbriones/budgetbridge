TARGET = budgetbridge

SRC := $(wildcard *.go)

all: $(TARGET)

$(TARGET): build

lint: $(SRC)
	go run honnef.co/go/tools/cmd/staticcheck
	go vet

test: $(SRC)
	go test -v

build: lint test
	go build
