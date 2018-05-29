ifeq ($(OS), Windows_NT)
	GOOS := windows
else
	UNAME := $(shell uname -s)
	ifeq ($(UNAME), Darwin)
		GOOS := darwin
	else
		GOOS := linux
	endif
endif

.PHONY: local-dir
local-dir:
	mkdir -p $(CURDIR)/local-config
	mkdir -p $(CURDIR)/local-log

.PHONY: install
install:
	$(CURDIR)/install.sh

.PHONY: binary
binary:
	bash $(CURDIR)/scripts/make_binary_with_container.sh $(USER) $(GOOS)

.PHONY: image
image:
	docker build --target metricproxy-image -t quay.io/signalfx/metricproxy:$(USER) .

.PHONY: run-image
run-image: local-dir
	docker run -ti \
	--rm \
	--name metricproxy-$(USER) \
	-p 12003:12003 \
	-p 18080:18080 \
	-p 6009:6009 \
	-v $(CURDIR)/local-log:/var/log/sfproxy \
	-v $(CURDIR)/local-config:/var/config/sfproxy \
	quay.io/signalfx/metricproxy:$(USER)

.PHONY: final
final:
	docker build --target metricproxy-final -t quay.io/signalfx/metricproxy:final .


	

