APP_NAME:=goprobe
APP_PATH:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
SCRIPT_PATH:=$(APP_PATH)/scripts
COMPILE_OUT:=$(APP_PATH)/bin/$(APP_NAME)

run:export EGO_DEBUG=true
run:
	@cd $(APP_PATH) && egoctl run
install:export EGO_DEBUG=true
install:
	@cd $(APP_PATH) && go run main.go --config=config/local.toml --job=install

build.darwin:
	@echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>making build app<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<"
	@chmod +x $(SCRIPT_PATH)/build/*.sh
	@cd $(APP_PATH) &&  GOOS=darwin $(SCRIPT_PATH)/build/gobuild.sh $(APP_NAME) $(COMPILE_OUT)

build:
	@echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>making build app<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<"
	@chmod +x $(SCRIPT_PATH)/build/*.sh
	@cd $(APP_PATH) && $(SCRIPT_PATH)/build/gobuild.sh $(APP_NAME) $(COMPILE_OUT)

