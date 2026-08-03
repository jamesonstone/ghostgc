// fixture-helper supplies deterministic native process behavior for live
// collector tests. It is intentionally tiny: --sleep blocks without periodic
// runtime housekeeping, --tick produces real file activity, --exit gives the
// parent a zombie, and --session execs the fixture root without a terminal.
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

static int session_exec(char **argv) {
	if (setsid() < 0) {
		perror("fixture-helper setsid");
		return 1;
	}
	execv(argv[2], &argv[2]);
	perror("fixture-helper execv");
	return 1;
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
	if (argc >= 3 && strcmp(argv[1], "--session") == 0) {
		return session_exec(argv);
	}
	fprintf(stderr, "usage: fixture-helper --exit | --sleep | --tick <path> | --session <command> [args]\n");
	return 2;
}
