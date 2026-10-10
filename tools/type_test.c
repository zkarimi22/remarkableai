// Slow, synchronized Linux uinput test for the reMarkable 2.
// Does not select a tool or move the cursor. Optional --body sends Ctrl+3 slowly.
#include <errno.h>
#include <fcntl.h>
#include <linux/input.h>
#include <linux/uinput.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h>
#include <unistd.h>

static int fd = -1;
static void cleanup(void) {
    if (fd >= 0) { ioctl(fd, UI_DEV_DESTROY); close(fd); }
}
static void fail(const char *what) { perror(what); exit(1); }
static void emit(unsigned short type, unsigned short code, int value) {
    struct input_event event = {0};
    event.type = type; event.code = code; event.value = value;
    if (write(fd, &event, sizeof event) != sizeof event) fail("write event");
}
static void key(unsigned short code, int value) {
    emit(EV_KEY, code, value);
    emit(EV_SYN, SYN_REPORT, 0);
}
int main(int argc, char **argv) {
    int body = argc == 2 && strcmp(argv[1], "--body") == 0;
    if (argc > 1 && !body) {
        fprintf(stderr, "Usage: %s [--body]\n", argv[0]);
        return 2;
    }
    const char *text = "hello from your mac.";
    const unsigned short letters[26] = {
        KEY_A, KEY_B, KEY_C, KEY_D, KEY_E, KEY_F, KEY_G, KEY_H, KEY_I,
        KEY_J, KEY_K, KEY_L, KEY_M, KEY_N, KEY_O, KEY_P, KEY_Q, KEY_R,
        KEY_S, KEY_T, KEY_U, KEY_V, KEY_W, KEY_X, KEY_Y, KEY_Z
    };
    fd = open("/dev/uinput", O_WRONLY);
    if (fd < 0) fail("open uinput");
    atexit(cleanup);
    if (ioctl(fd, UI_SET_EVBIT, EV_KEY) < 0) fail("EV_KEY");
    for (int i = 0; i < 26; i++)
        if (ioctl(fd, UI_SET_KEYBIT, letters[i]) < 0) fail("letter key");
    if (ioctl(fd, UI_SET_KEYBIT, KEY_SPACE) < 0) fail("space key");
    if (ioctl(fd, UI_SET_KEYBIT, KEY_DOT) < 0) fail("dot key");
    if (body) {
        if (ioctl(fd, UI_SET_KEYBIT, KEY_LEFTCTRL) < 0) fail("ctrl key");
        if (ioctl(fd, UI_SET_KEYBIT, KEY_3) < 0) fail("style key");
    }
    struct uinput_user_dev device = {0};
    snprintf(device.name, sizeof device.name, "Remarkable slow typing test");
    device.id.bustype = BUS_USB;
    device.id.vendor = 0x1234; device.id.product = 1; device.id.version = 1;
    if (write(fd, &device, sizeof device) != sizeof device) fail("device setup");
    if (ioctl(fd, UI_DEV_CREATE) < 0) fail("create keyboard");
    puts("Keyboard connected. You have 15 seconds to place the text cursor on your test page.");
    fflush(stdout);
    sleep(15);
    if (body) {
        key(KEY_LEFTCTRL, 1); usleep(150000);
        key(KEY_3, 1); usleep(80000);
        key(KEY_3, 0); usleep(150000);
        key(KEY_LEFTCTRL, 0); usleep(750000);
    }
    for (const char *p = text; *p; ++p) {
        unsigned short code = *p == ' ' ? KEY_SPACE : *p == '.' ? KEY_DOT : letters[*p - 'a'];
        key(code, 1);
        usleep(80000);
        key(code, 0);
        usleep(250000);
    }
    sleep(2);
    puts("Finished. Expected text: hello from your mac.");
    return 0;
}
