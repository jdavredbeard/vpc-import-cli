VERSION := 0.1.1
BUILD := $(shell git rev-parse HEAD)
SRCDIRS := .
BINDIR := ./bin
CICD-GOOS :="linux" 
CICD-GOARCH :="amd64"
APP-NAME := "vpc-import-cli"

.PHONY: all-cicd all-local build-cicd build-local clean cleangen test install

all-cicd: test build-cicd
all-local: test build-local
build-cicd:
	@for srcdir in $(SRCDIRS); do env GOOS=$(CICD-GOOS) GOARCH=$(CICD-GOARCH) go build -o $(BINDIR)/$(APP-NAME)-$(VERSION)-$(CICD-GOARCH)-$(CICD-GOOS) $$srcdir; done;
build-local:
	@for srcdir in $(SRCDIRS); do go build -o $(BINDIR)/$(APP-NAME) $$srcdir; done;
clean:
	@rm -vf $(BINDIR)/*
cleangen:
	@rm -vf terraform.tfstate
	@rm -vf terraform.tfstate.backup
	@rm -vf .terraform.lock.hcl
	@rm -vf terraform.tfvars.json
test:
	@for srcdir in $(SRCDIRS); do go test -v $$srcdir; done;
install:
	@for srcdir in $(SRCDIRS); do go install $$srcdir; done;
cicd-release: all-cicd
	@gh release create $(APP-NAME)-$(VERSION)-amd64-linux --generate-notes
	@gh release upload $(APP-NAME)-$(VERSION)-amd64-linux bin/$(APP-NAME)-$(VERSION)-amd64-linux
