#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <arpa/inet.h>
#include <sys/socket.h>
#include <pthread.h>
#include "raptor_api.h"

#define MAX_NUM_PATH		2 
#define UPPER_LAYER			999
#define BUF_SIZE 			1460
#define SOCKET_TIMEOUT		30
#define THREAD_STACK_SIZE 	15 * 1024 * 1024
#define TOTAL_BUCKET		100
#define THROUGHPUT_WINDOW	5 * 1000 * 1000		// Mbps
#define TIME_WINDOW			0.5		// second

typedef struct {
    char ip[16];
    uint16_t port;
} addr_info_t;

typedef struct { 
	int fec_mode;
	int receiver_mode;
	int num_paths;

	addr_info_t app_src_addr; 
	addr_info_t app_dst_addr;
	addr_info_t tun_src_addr[MAX_NUM_PATH];
    addr_info_t tun_dst_addr[MAX_NUM_PATH];

	struct sockaddr_in app_dst;
	struct sockaddr_in tun_src[MAX_NUM_PATH];
	struct sockaddr_in tun_dst[MAX_NUM_PATH];

	int app_sock;
	int tun_sock[MAX_NUM_PATH];  

	int secondary_path;

	uint32_t total_sent_pkts[MAX_NUM_PATH];
	uint32_t total_sent_bytes[MAX_NUM_PATH]; 
	int throughput[MAX_NUM_PATH];
	int pkt_bucket[MAX_NUM_PATH];
	double start_time[MAX_NUM_PATH];
} fectun_cfg_t; 

fectun_cfg_t *get_options(int argc, char *argv[]);
void parse_addr_info(char *input, addr_info_t *info);
void print_usage(); 

void network_init();
int  socket_init(addr_info_t *src_addr); //, addr_info_t *dst_addr);
void tunnel_start();
void *tunnel_recv_thread(void *arg);
void *send_encoded_data(); 
void *send_decoded_data();
void send_raptor_packet(uint32_t type, uint32_t packet_seq, uint32_t block_seq, 
						uint32_t symbol_seq, uint32_t start_seq, uint32_t end_seq, 
						char *data, uint32_t len);
void recv_raptor_packet(raptor_packet_t *packet);

int packet_scheduling();
void path_selection();
void update_measurement(int path, uint32_t sent_bytes);

void error_handling(char *message);
