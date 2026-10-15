ARG GO_IMAGE=golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
FROM ${GO_IMAGE} AS source
WORKDIR /src
ENV GOTOOLCHAIN=local
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY integration ./integration
COPY rules ./rules
COPY testdata ./testdata

# A build ancestor: BuildKit cannot skip these checks.
FROM source AS test
RUN CGO_ENABLED=1 go test -count=1 -race -timeout 180s ./... && go vet ./...

FROM test AS build
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/client ./cmd/client

FROM scratch AS server
COPY --from=build /out/server /server
COPY --from=build /src/rules/fingerprints.json /rules/fingerprints.json
USER 65532:65532
EXPOSE 8080
STOPSIGNAL SIGTERM
ENTRYPOINT ["/server"]
CMD ["--addr", ":8080", "--rules", "/rules/fingerprints.json"]

FROM scratch AS client
COPY --from=build /out/client /client
USER 65532:65532
ENTRYPOINT ["/client"]
CMD ["--input", "/data/scans.json", "--server", "http://server:8080"]
