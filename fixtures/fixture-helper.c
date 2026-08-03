// fixture-helper supplies deterministic native process behavior for live
// collector tests. It is intentionally tiny: --sleep blocks without periodic
// runtime housekeeping, --tick produces real file activity, and --exit gives
// the parent a zombie to observe.
#include <fcntl.h>
#include <stdio.h>
#include <string.h>
#include <time.h>
#include <unistd.h>

static int tick(const char *path) {
	for (;;) {
		struct timespec now;
		char body[64];
		if (clock_gettime(CLOCK_REALTIME, &now) != 0) {
			return 1;
		}
		int size = snprintf(body, sizeof(body), "%lld.%09ld\n",
			(long long)now.tv_sec, now.tv_nsec);
		int fd = open(path, O_WRONLY | O_CREAT | O_TRUNC, 0600);
		if (fd < 0 || write(fd, body, (size_t)size) != size) {
			if (fd >= 0) {
				close(fd);
			}
			return 1;
		}
		close(fd);
		sleep(2);
	}
}

int main(int argc, char **argv) {
	if (argc == 2 && strcmp(argv[1], "--exit") == 0) {
		return 0;
	}
	if (argc == 2 && strcmp(argv[1], "--sleep") == 0) {
		for (;;) {
			pause();
		}
	}
	if (argc == 3 && strcmp(argv[1], "--tick") == 0) {
		return tick(argv[2]);
	}
	fprintf(stderr, "usage: fixture-helper --exit | --sleep | --tick <path>\n");
	return 2;
}
