#ifndef UTIL_H
#define UTIL_H

#include <stdio.h>
#include <stdlib.h>
#include <stdarg.h>
#include <stdalign.h>
#include <string.h>
#include <unistd.h>
#include <signal.h>
#include <sys/time.h>
#include <time.h>

double get_timestamp();
static void timer_handler(int sig, siginfo_t *si, void *uc);
int start_timer(timer_t *timer_id, int expire_time);
void print_log(const char *format, ...);
void print_hex(const char *data, int len);
void print_hex_2d(char **data, int row, int col);

#endif
