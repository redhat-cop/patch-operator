FROM registry.access.redhat.com/ubi9/ubi-minimal@sha256:d235f607e1d6d833f031db107dc42206e4dd4d5aa9142c43d3771fb7f9bea76a
WORKDIR /
COPY bin/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
