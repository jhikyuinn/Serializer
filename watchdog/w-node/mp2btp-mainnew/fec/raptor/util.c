#include "util.h"
#include <pthread.h>


double get_timestamp()
{
	struct timeval cur_time;
	double sec;

	gettimeofday(&cur_time, NULL);
	sec = cur_time.tv_sec - (((int)(cur_time.tv_sec/1000000.0))*1000000.0) + cur_time.tv_usec/1000000.0;

	return sec;
}


static void timer_handler(int sig, siginfo_t *si, void *uc)
{
    //timer_t *tidp;
    
    // tidp = (void**)(si->si_value.sival_ptr);

    // if ( *tidp == firstTimerID )
    //     printf("2ms\n");
    // else if ( *tidp == secondTimerID )
    //     printf("10ms\n");
    // else if ( *tidp == thirdTimerID )

    // Decoding Case #3: Timer is expired
    printf("100ms Timer is expired!!!!!! \n");

    // TODO: implement 
}


int start_timer(timer_t *timer_id, int expire_time)
{
    struct sigevent te;
    struct itimerspec its;
    struct sigaction sa;
    int sigNo = SIGRTMIN;

    // Set up signal handler. 
    sa.sa_flags = SA_SIGINFO;
    sa.sa_sigaction = timer_handler;
    sigemptyset(&sa.sa_mask);
    if (sigaction(sigNo, &sa, NULL) == -1) 
    {
        perror("sigaction");
    }

    // Set and enable alarm 
    te.sigev_notify = SIGEV_SIGNAL;
    te.sigev_signo = sigNo;
    te.sigev_value.sival_ptr = timer_id;

    timer_create(CLOCK_REALTIME, &te, timer_id);

    // its.it_interval.tv_sec = 0;
    // its.it_interval.tv_nsec = intervalMS * 1000000;
    its.it_value.tv_sec = 0;
    its.it_value.tv_nsec = expire_time * 1000000;  

    timer_settime(*timer_id, 0, &its, NULL);

    return 1;
}


void print_log(const char *format, ...)
{
    static int init = 0;
    static pthread_mutex_t mutex;

    if (init == 0) 
    {
        pthread_mutex_init(&mutex, NULL);
        init = 1;
    }

    va_list args; 
    va_start(args, format);
    pthread_mutex_lock(&mutex);
    printf("[%.4f] ", get_timestamp()); 
    vprintf(format, args);
    printf("\n");
    fflush(stdout);
    pthread_mutex_unlock(&mutex);
    va_end(args);
}


void print_hex(const char *data, int len) 
{
    int i; 
    static int idx = 0;

    // Head: first 
    if (len > 32 * 5)
    {
        printf("Data %dth (len=%d):\n", idx++, len);
        for (i = 0; i < 32 * 5; i++)
        {
            printf("%02x ", data[i] & 0xff);
            if ((i+1) % 32 == 0)
                printf("\n");
        }
    }
    else 
    {
        print_log("print_hex() error: data length < 32 (len=%d)", len);
    }
    printf("\n");
}

void print_hex_2d(char **data, int row, int col)
{
    int i, j; 
    static int idx = 0;

    printf("2D Data %dth (%dx%d):\n", idx++, row, col);
    for (i = 0; i < row; i++) 
    {
        printf("%02d: ", i);
        for (j = 0; j < col; j++) 
        {
            printf("%02x ", data[i][j] & 0xff);
        }
        printf("\n");
    }
}



// void log_format(const char* tag, const char* message, va_list args) {   
//     time_t now;     
//     time(&now);     
//     char * date =ctime(&now);   
//     date[strlen(date) - 1] = '\0';  
//     printf("%s [%s] ", date, tag);  
//     vprintf(message, args);     
//     printf("\n"); 
// }


// void log_debug(const char* message, ...) 
// { 
//     va_list args;   
//     va_start(args, message);    
//     log_format("debug", message, args);     
//     va_end(args); 
// }
