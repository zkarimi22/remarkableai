# Security

Please do not report API keys, SSH passwords, private notebook captures, or other credentials in a public issue. Remove them from logs and screenshots before sharing diagnostics.

For a suspected security issue, contact the repository owner privately before opening a public issue. Include the affected component, firmware version, reproduction steps that do not expose credentials, and the smallest useful log excerpt.

The assistant stores the OpenRouter key only on the tablet in a mode-600 configuration file. It does not expose an inbound network listener. The OpenRouter service still receives the captured notebook page; do not use it for confidential content unless that data flow is acceptable to you.
