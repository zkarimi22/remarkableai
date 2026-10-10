// Slow, synchronized Linux uinput test for the reMarkable 2.
// Reads ASCII text from stdin; selects body text using slow Ctrl+3.
// --erase N deletes exactly N characters from the end of the text after caller guards.
#include <errno.h>
#include <fcntl.h>
#include <linux/input.h>
#include <linux/uinput.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h>
#include <unistd.h>

static int fd = -1;
static volatile sig_atomic_t stopping = 0;
static unsigned char held[KEY_MAX + 1];
static void stop_typing(int signal_number) { (void)signal_number; stopping = 1; }
static void cleanup(void) {
    if (fd >= 0) {
        struct input_event event = {0};
        event.type = EV_KEY;
        for (unsigned short code = 0; code <= KEY_MAX; code++) {
            if (held[code]) {
                event.code = code;
                (void)write(fd, &event, sizeof event);
            }
        }
        event.type = EV_SYN; event.code = SYN_REPORT;
        (void)write(fd, &event, sizeof event);
        ioctl(fd, UI_DEV_DESTROY); close(fd);
    }
}
static void fail(const char *what) { perror(what); exit(1); }
static void emit(unsigned short type, unsigned short code, int value) {
    struct input_event event = {0};
    event.type = type; event.code = code; event.value = value;
    if (write(fd, &event, sizeof event) != sizeof event) fail("write event");
}
static void key(unsigned short code, int value) {
    if (stopping && value) return;
    emit(EV_KEY, code, value);
    emit(EV_SYN, SYN_REPORT, 0);
    held[code] = value != 0;
}
static void body_style(void) {
    key(KEY_LEFTCTRL, 1); usleep(150000);
    key(KEY_3, 1); usleep(80000);
    key(KEY_3, 0); usleep(150000);
    key(KEY_LEFTCTRL, 0); usleep(750000);
}
int main(int argc, char **argv) {
    unsigned long erase_count = 0;
    unsigned long delay_ms = 180;
    int replace_pending = 0;
    int clear_pending = 0;
    for (int arg = 1; arg < argc; arg++) {
        if (strcmp(argv[arg], "--replace-pending") == 0) {
            replace_pending = 1;
            continue;
        }
        if (strcmp(argv[arg], "--clear-pending") == 0) {
            clear_pending = 1;
            continue;
        }
        char *end = NULL;
        if (arg + 1 >= argc || (strcmp(argv[arg], "--erase") != 0 && strcmp(argv[arg], "--delay-ms") != 0)) {
            fputs("Usage: type-text-rm2 [--erase character-count | --delay-ms 100..500 [--replace-pending]]\n", stderr);
            return 2;
        }
        errno = 0;
        int erase = strcmp(argv[arg], "--erase") == 0;
        const char *value_string = argv[++arg];
        unsigned long value = strtoul(value_string, &end, 10);
        if (errno || end == value_string || *end || value < (erase ? 1UL : 100UL) || value > (erase ? 12000UL : 500UL)) {
            fputs("Invalid character count or typing delay.\n", stderr);
            return 2;
        }
        if (erase) erase_count = value;
        else delay_ms = value;
    }
    if (erase_count && (replace_pending || clear_pending)) {
        fputs("Cannot combine --erase and pending-indicator options.\n", stderr);
        return 2;
    }
    if (replace_pending && clear_pending) {
        fputs("Cannot combine --replace-pending and --clear-pending.\n", stderr);
        return 2;
    }
    signal(SIGTERM, stop_typing);
    signal(SIGINT, stop_typing);
    char text[16385];
    size_t length = erase_count ? 0 : fread(text, 1, sizeof(text) - 1, stdin);
    if (!erase_count && !clear_pending && (ferror(stdin) || !feof(stdin) || length == 0)) {
        fputs("Expected 1-16383 bytes of ASCII text on stdin.\n", stderr);
        return 2;
    }
    text[length] = 0;
    for (size_t i = 0; i < length; i++) {
        unsigned char c = (unsigned char)text[i];
        if (c != '\n' && (c < 32 || c > 126)) {
            fputs("Unsupported character; use plain ASCII text.\n", stderr);
            return 2;
        }
    }
    const unsigned short letters[26] = {
        KEY_A, KEY_B, KEY_C, KEY_D, KEY_E, KEY_F, KEY_G, KEY_H, KEY_I,
        KEY_J, KEY_K, KEY_L, KEY_M, KEY_N, KEY_O, KEY_P, KEY_Q, KEY_R,
        KEY_S, KEY_T, KEY_U, KEY_V, KEY_W, KEY_X, KEY_Y, KEY_Z
    };
    unsigned short codes[128] = {0};
    unsigned char shifts[128] = {0};
    for (int i = 0; i < 26; i++) {
        codes['a' + i] = codes['A' + i] = letters[i];
        shifts['A' + i] = 1;
    }
    const char *plain = "1234567890-=[]\\;',./`";
    const char *shifted = "!@#$%^&*()_+{}|:\"<>?~";
    const unsigned short punctuation[] = {KEY_1,KEY_2,KEY_3,KEY_4,KEY_5,
        KEY_6,KEY_7,KEY_8,KEY_9,KEY_0,KEY_MINUS,KEY_EQUAL,KEY_LEFTBRACE,
        KEY_RIGHTBRACE,KEY_BACKSLASH,KEY_SEMICOLON,KEY_APOSTROPHE,
        KEY_COMMA,KEY_DOT,KEY_SLASH,KEY_GRAVE};
    for (size_t i = 0; plain[i]; i++) {
        codes[(unsigned char)plain[i]] = codes[(unsigned char)shifted[i]] = punctuation[i];
        shifts[(unsigned char)shifted[i]] = 1;
    }
    codes[' '] = KEY_SPACE; codes['\n'] = KEY_ENTER;
    fd = open("/dev/uinput", O_WRONLY);
    if (fd < 0) fail("open uinput");
    atexit(cleanup);
    if (ioctl(fd, UI_SET_EVBIT, EV_KEY) < 0) fail("EV_KEY");
    for (int i = 0; i < 128; i++)
        if (codes[i] && ioctl(fd, UI_SET_KEYBIT, codes[i]) < 0) fail("key capability");
    if (ioctl(fd, UI_SET_KEYBIT, KEY_LEFTCTRL) < 0) fail("ctrl key");
    if (ioctl(fd, UI_SET_KEYBIT, KEY_LEFTSHIFT) < 0) fail("shift key");
    if (erase_count || replace_pending || clear_pending) {
        if (ioctl(fd, UI_SET_KEYBIT, KEY_END) < 0) fail("end key");
        if (ioctl(fd, UI_SET_KEYBIT, KEY_BACKSPACE) < 0) fail("backspace key");
    }
    struct uinput_user_dev device = {0};
    snprintf(device.name, sizeof device.name, "Remarkable AI typing");
    device.id.bustype = BUS_USB;
    device.id.vendor = 0x1234; device.id.product = 1; device.id.version = 1;
    if (write(fd, &device, sizeof device) != sizeof device) fail("device setup");
    if (ioctl(fd, UI_DEV_CREATE) < 0) fail("create keyboard");
    puts(erase_count ? "Removing last reply. Keep this page open." : "Typing reply shortly. Keep this notebook page open.");
    fflush(stdout);
    sleep(2);
    if (stopping) return 130;
    if (replace_pending || clear_pending) {
        // Caller checks that no touch, writing, or page change occurred since
        // typing '...'. Keep the caret in place; do not use the undo shortcut.
        for (int i = 0; i < 3 && !stopping; i++) {
            key(KEY_BACKSPACE, 1); usleep(80000);
            key(KEY_BACKSPACE, 0); usleep(250000);
        }
    }
    if (erase_count) {
        // The gesture itself may move the caret. Return to the text end first.
        key(KEY_LEFTCTRL, 1); usleep(150000);
        key(KEY_END, 1); usleep(80000);
        key(KEY_END, 0); usleep(150000);
        key(KEY_LEFTCTRL, 0); usleep(750000);
        for (unsigned long i = 0; i < erase_count && !stopping; i++) {
            key(KEY_BACKSPACE, 1); usleep(80000);
            key(KEY_BACKSPACE, 0); usleep(250000);
        }
        if (stopping) return 130;
        sleep(2);
        puts("Last reply removed.");
        return 0;
    }
    int paragraph_start = 1;
    for (const char *p = text; *p && !stopping; ++p) {
        unsigned char c = (unsigned char)*p;
        unsigned short code = codes[c];
        if (shifts[c]) { key(KEY_LEFTSHIFT, 1); usleep(80000); }
        key(code, 1);
        usleep(80000);
        key(code, 0);
        if (shifts[c]) { usleep(80000); key(KEY_LEFTSHIFT, 0); }
        usleep(delay_ms * 1000);
        // Enter establishes text focus and creates the new paragraph. Applying
        // style before that can affect the preceding paragraph or be ignored.
        // Reapply after its first character as well, when it is no longer empty.
        if (c == '\n' || paragraph_start) body_style();
        paragraph_start = c == '\n';
    }
    if (stopping) return 130;
    sleep(2);
    puts("Reply typed.");
    return 0;
}
