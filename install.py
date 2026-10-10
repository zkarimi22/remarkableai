"""Install the standalone Wi-Fi assistant on the USB-connected reMarkable 2."""
import base64
import argparse
import getpass
import json
from pathlib import Path
import subprocess
import tempfile
import sys
import shlex

ROOT = Path(__file__).resolve().parent
TABLET = "root@10.11.99.1"
REMOTE = "/home/root/.local/remarkableai"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--riddle", action="store_true", help="apply DeepSeek V4.1 Flash, Tom Riddle persona, web search, and tighter spacing")
    args = parser.parse_args()
    files = [ROOT / "bin/remarkableai-rm2", ROOT / "bin/type-text-rm2", ROOT / "remarkableai.service"]
    for path in files:
        if not path.is_file():
            raise RuntimeError(f"Missing build artifact: {path}")
    print("Install the Wi-Fi assistant. Keep USB connected during installation.")
    print("Your OpenRouter key will be stored privately on the tablet, not on this Mac.")
    key = getpass.getpass("OpenRouter API key (hidden; blank keeps an existing tablet key): ").strip()
    with tempfile.TemporaryDirectory(prefix="rmai-install-") as folder:
        options = ["-o", "ControlPath=" + str(Path(folder) / "ssh")]
        def remote(command, **kwargs):
            return subprocess.run(["ssh", *options, TABLET, command], check=True, **kwargs)
        subprocess.run(["ssh", *options, "-o", "ControlMaster=yes", "-o",
                        "ControlPersist=120", "-fN", TABLET], check=True)
        try:
            remote("test -x /home/root/ghostwriter-test")
            if not key:
                remote("test -s /home/root/.config/remarkableai/config.json")
            remote("set -e; if test -f /etc/systemd/system/remarkableai.service; then "
                   "systemctl stop remarkableai.service; fi; "
                   "mkdir -p /home/root/.local/remarkableai /home/root/.config/remarkableai; "
                   "chmod 700 /home/root/.local/remarkableai /home/root/.config/remarkableai")
            for source, target in zip(files, ["remarkableai", "type-text-rm2", "remarkableai.service"]):
                subprocess.run(["scp", "-O", *options, str(source), TABLET+":"+REMOTE+"/"+target], check=True)
            if key:
                config = json.dumps({"api_key": key, "model": "google/gemini-2.5-flash", "timezone": "America/Vancouver"})
                remote("umask 077; cat > /home/root/.config/remarkableai/config.json.new && "
                       "mv /home/root/.config/remarkableai/config.json.new /home/root/.config/remarkableai/config.json",
                       input=config.encode())
            # This placeholder only lets Ghostwriter run its cursor-positioning step.
            png = base64.b64decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aV1sAAAAASUVORK5CYII=")
            remote("umask 077; cat > " + REMOTE + "/placeholder.png", input=png)
            remote("set -e; chmod 700 " + REMOTE + "/remarkableai " + REMOTE + "/type-text-rm2; "
                   "chmod 600 /home/root/.config/remarkableai/config.json")
            if args.riddle:
                remote(shlex.join([REMOTE + "/remarkableai",
                    "--set-model", "deepseek/deepseek-v4.1-flash",
                    "--set-system-instructions", "You are Tom Riddle, a student at Hogwarts, speaking through a notebook assistant. Always answer as Tom Riddle, the Hogwarts student.",
                    "--set-web-search", "on", "--set-leading-lines", "2"]))
            print("Checking the standalone screen capture...", flush=True)
            remote(REMOTE + "/remarkableai --capture " + REMOTE + "/diagnostic.png")
            artifacts = ROOT / "artifacts"
            artifacts.mkdir(exist_ok=True)
            subprocess.run(["scp", "-O", *options, TABLET+":"+REMOTE+"/diagnostic.png",
                            str(artifacts / "tablet-capture.png")], check=True)
            remote("cp " + REMOTE + "/remarkableai.service /etc/systemd/system/remarkableai.service && "
                   "chmod 644 /etc/systemd/system/remarkableai.service && "
                   "systemctl daemon-reload && systemctl enable --now remarkableai.service && "
                   "sleep 1 && systemctl is-active --quiet remarkableai.service && "
                   "cat /run/remarkableai-status")
            print("\nInstalled. Connect the tablet to Wi-Fi, open a notebook question, and unplug USB.")
            print("Double-tap with your finger within about 1 cm of the lower-right screen corner.")
            print("Keep the page still while waiting and typing. Touching or writing cancels the reply.")
            print("Three dots mean the model is working; they are replaced by the answer.")
            if args.riddle:
                print("DeepSeek V4.1 Flash + Tom Riddle + web search + PDF notes enabled.")
                print("PDF notes are placed in the fixed AI Notes folder; create-note requests restart Xochitl briefly.")
            print("To undo the latest completed reply, double-tap about 2 cm from the left and bottom screen edges.")
            print("Undo is available until you edit, navigate, or restart the assistant.")
            print("To stop later: ssh root@10.11.99.1 'systemctl stop remarkableai'")
        finally:
            subprocess.run(["ssh", *options, "-O", "exit", TABLET],
                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\nInstallation interrupted.", file=sys.stderr)
        sys.exit(130)
    except (OSError, RuntimeError, subprocess.CalledProcessError) as error:
        print("Installation stopped:", error, file=sys.stderr)
        sys.exit(1)
