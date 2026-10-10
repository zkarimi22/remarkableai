# Contributing

This project controls a real reMarkable tablet. Keep changes small, reversible, and testable without a tablet.

Before opening a pull request:

```sh
make test
make build
```

For device changes, test first with a disposable notebook. Do not include API keys, SSH passwords, tablet captures, or personal notebook files in a commit. Keep device-specific behavior behind small functions with unit tests where possible.

Pull requests should explain the firmware tested, the hardware path used (USB or Wi-Fi), and any behavior that still needs hardware verification.
