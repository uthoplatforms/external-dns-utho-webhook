FROM gcr.io/distroless/static-debian11:nonroot

USER 20000:20000
ADD --chmod=555 external-dns-utho-webhook /opt/external-dns-utho-webhook/app

ENTRYPOINT ["/opt/external-dns-utho-webhook/app"]