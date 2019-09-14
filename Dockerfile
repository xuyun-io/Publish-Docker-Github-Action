FROM golang:1.13.0-alpine as build
ADD . / src/
ENV GOOS=linux GARCH=amd64 CGO_ENABLED=0 GOFLAGS='-mod=vendor'
RUN cd src \
 && go test ./... \
 && go build -o /entrypoint entrypoint.go \
 && chmod +x /entrypoint

FROM docker:19.03.1 as runtime
LABEL "com.github.actions.name"="Publish Docker"
LABEL "com.github.actions.description"="Uses the git branch as the docker tag and pushes the container"
LABEL "com.github.actions.icon"="anchor"
LABEL "com.github.actions.color"="blue"

LABEL "repository"="https://github.com/elgohr/Publish-Docker-Github-Action"
LABEL "maintainer"="Lars Gohr"

RUN apk update \
  && apk upgrade \
  && apk add --no-cache git

COPY --from=build /entrypoint /entrypoint
ENTRYPOINT ["/entrypoint"]
