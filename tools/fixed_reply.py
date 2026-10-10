"""Local-only Ghostwriter text insertion test; no AI service is contacted."""

import json
from http.server import BaseHTTPRequestHandler, HTTPServer


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path != "/v1/chat/completions":
            self.send_error(404)
            return
        length = int(self.headers.get("Content-Length", "0"))
        if not 0 < length <= 16 * 1024 * 1024:
            self.send_error(413)
            return
        self.rfile.read(length)
        payload = json.dumps({
            "choices": [{"message": {
                "role": "assistant",
                "content": None,
                "tool_calls": [{
                    "id": "fixed-reply-test",
                    "type": "function",
                    "function": {
                        "name": "draw_text",
                        "arguments": json.dumps({"text": "Hello from your Mac."}),
                    },
                }],
            }, "finish_reason": "tool_calls"}],
        }).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)


if __name__ == "__main__":
    print("Fixed reply ready on 127.0.0.1:18765", flush=True)
    HTTPServer(("127.0.0.1", 18765), Handler).serve_forever()
