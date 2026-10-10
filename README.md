# remarkableai

write a question on a reMarkable 2. double-tap the lower-right corner. the answer gets typed onto the same page. 

it runs over wifi after setup. your mac is only needed for install/update.

currently uses a tom riddle persona in the system prompt because i was watching harry potter with my wife and daughter but you can change that easily. 

2 main types of getting ai responses: either typing the question directly OR starting the page with "create a note for ___" which will create a long form separate PDF file based on your query for example "Create a note for the biography of Alexander the Great compared to Napolean"



## setup

needs:

- rooted reMarkable 2
- firmware 3.24+ (tested on 3.28.0.172)
- USB connection for setup
- an [OpenRouter](https://openrouter.ai/) API key
- Ghostwriter installed as `/home/root/ghostwriter-test`

on your mac:

```sh
git clone https://github.com/<you>/remarkableai.git
cd remarkableai
brew install go zig
make test
make build
python3 install.py --riddle
```

the installer asks for your OpenRouter key and copies everything to the tablet. press enter at the key prompt to keep the existing key.

## use

1. connect the reMarkable to wifi
2. open a notebook page
3. write your question
4. double-tap near the lower-right corner

`...` means it is working. touch or write to cancel.

simple questions stay short. longer requests can continue across pages.

## pdf notes

write:

```text
create a note for Alexander the Great
```

the tablet creates a PDF in the `AI Notes` folder. xochitl restarts briefly while it appears. try it in a disposable notebook first.

## settings

the default model is DeepSeek V4.1 Flash. `--riddle` uses the Tom Riddle, Hogwarts student persona and web search.

to change typing speed or spacing:

```sh
ssh root@10.11.99.1 '/home/root/.local/remarkableai/remarkableai \
  --set-typing-delay 180 --set-leading-lines 2 \
  --set-web-search on && systemctl restart remarkableai'
```

## develop

```sh
make test
make build
```

firmware updates may break capture, input, or PDF integration. webpage screenshots are not implemented yet. the API key stays on the tablet, but page images are sent to OpenRouter.

see [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md), [THIRD_PARTY.md](THIRD_PARTY.md), and [LICENSE](LICENSE).
