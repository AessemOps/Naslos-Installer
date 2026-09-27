# installpack (embedded install pack)

This directory is the `go:embed` target for the versioned **Naslos install
pack** (`naslos-install-pack-<version>.tar.gz`, published by the
`AessemOps/Naslos-Linux` `install-pack` release workflow).

It is gitignored on purpose — packs are build artifacts, not source:

```bash
make fetch-pack PACK_VERSION=0.1.0     # download + sha256-verify + extract here
```

Only this `README.md` and `embed.go` are committed, so `//go:embed *` always
matches at least one file on a fresh checkout. After a fetch the directory
contains:

```
metadata.json
charts/naslos/...
machine-config/naslos-installer.yaml.tmpl
cilium/cilium.yaml
manifests/local-path-v0.0.26.yaml
schematic/naslos.yaml
```

See [`docs/installer-contract.md`](https://github.com/AessemOps/Naslos-Linux/blob/master/docs/installer-contract.md)
in Naslos-Linux for the pack schema and the stable interfaces. The Go loader
lives in `internal/installpack`.
