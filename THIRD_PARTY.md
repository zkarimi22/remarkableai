Capture and input compatibility references:

- [goMarkableStream v1.4.0](https://github.com/owulveryck/goMarkableStream/tree/v1.4.0): reMarkable 2 firmware 3.24+ framebuffer layout, offset, and portrait dimensions.
- [Ghostwriter v2025.09.27-01](https://github.com/awwaiid/ghostwriter/tree/v2025.09.27-01): existing installed executable used for cursor positioning only; touch coordinate orientation and Ctrl+3 body-text shortcut.
- `device/ca-certificates.pem`: public CA bundle copied from this Mac's `/etc/ssl/cert.pem`, used to verify HTTPS on the tablet. No private keys are included.

The Go and Zig toolchains are downloaded into `/tmp/remarkableai-toolchain`, not installed globally.
