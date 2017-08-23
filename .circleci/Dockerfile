# Generates an image with Golang and Ruby (for testing)
FROM circleci/ruby:2.4.1

RUN sudo apt-get update && \
	sudo apt-get install -y software-properties-common && \
	sudo apt-add-repository 'deb http://ppa.launchpad.net/longsleep/golang-backports/ubuntu xenial main' && \
	sudo apt-get update && sudo apt-get install golang-go=2:1.8~1ubuntu2~xenial

# Make go binaries available from our path
ENV PATH $PATH:/home/circleci/go/bin
RUN go get -u github.com/golang/dep/cmd/dep
