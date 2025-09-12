#include "fectun.h"

fectun_cfg_t *fectun; 


int main(int argc, char *argv[])
{
	// Get options 
	fectun = get_options(argc, argv); 
	
	// Raptor code init
	raptor_init(send_encoded_data, send_decoded_data); 

	// Network init
	network_init(argc, argv);

	// Tunnel start 
	tunnel_start(); 

	return 0;
}

fectun_cfg_t *get_options(int argc, char *argv[]) 
{
	int i; 
	int opt; 
	int a_count, u_count, s_count, d_count; 
	a_count = u_count = s_count = d_count = 0; 
	
	fectun_cfg_t *cfg = (fectun_cfg_t *) malloc(sizeof(fectun_cfg_t));

	cfg->fec_mode = 0;
	cfg->receiver_mode = 0;
	
	while ((opt = getopt(argc, argv, "fra:u:s:d:")) != -1)  
    { 
        switch (opt)  
        {  
            case 'f': 	// FEC mode
                cfg->fec_mode = 1;
                break;  
            case 'r':	// receiver mode 
				cfg->receiver_mode = 1;    
                break; 
			case 'a': 	// application source address 
				parse_addr_info(optarg, &cfg->app_src_addr);
				a_count = 1;
				break;
			case 'u': 	// application destination address
				parse_addr_info(optarg, &cfg->app_dst_addr);
				u_count = 1;
				break;
			case 's':	// tunnel source address
                if (s_count < 2) 
                    parse_addr_info(optarg, &cfg->tun_src_addr[s_count++]);
                break;
            case 'd':	// tunnel destination address
                if (d_count < 2)
                    parse_addr_info(optarg, &cfg->tun_dst_addr[d_count++]);
                break;
			default: 
				print_usage();
        } 
    }  

	cfg->num_paths = max(s_count, d_count);
	if (cfg->num_paths <= 0 || cfg->num_paths >= 3)
	{
		print_usage();
		error_handling("The number of paths must be either 1 or 2."); 
	}

	if (a_count == 0 || u_count == 0 || s_count == 0 || d_count == 0)
	{
		print_usage();
		error_handling("Please check options");
	}

	// Print parsed options 
	print_log("fec_mode=%d, receiver_mode=%d, num_paths=%d", 
				cfg->fec_mode, cfg->receiver_mode, cfg->num_paths);
	print_log("App Source Address=%s:%d", cfg->app_src_addr.ip, cfg->app_src_addr.port);		
	print_log("App Destination Address=%s:%d", cfg->app_dst_addr.ip, cfg->app_dst_addr.port);		
	for (i = 0; i < s_count; i++) 
		print_log("Tunnel Source Address[%d]=%s:%d", i, cfg->tun_src_addr[i].ip, cfg->tun_src_addr[i].port);
	for (i = 0; i < d_count; i++) 
		print_log("Tunnel Destination Address[%d]=%s:%d", i, cfg->tun_dst_addr[i].ip, cfg->tun_dst_addr[i].port);

	return cfg; 
}

void parse_addr_info(char *input, addr_info_t *info) 
{
    char *colon_pos = strchr(input, ':');
    if (colon_pos != NULL) 
	{
        *colon_pos = '\0'; 
        strcpy(info->ip, input);
        info->port = (uint16_t)atoi(colon_pos + 1);
    }
}

void print_usage() 
{ 
    printf("Usage: ./fectun [OPTION] -a [Address] -s [Address] ... -d [Address] ...\n" 
           "\t -f: FEC mode (no flag is Non-FEC mode) \n" 
           "\t -r: Receiver mode (cannot be chosen in the sender mode)\n"
		   "\t -a [Address]: App source Address (Address for receiving packet from upper layer_ \n"
		   "\t -u [Address]: App destination address (Address for receiving packet from lower layer \n"
		   "\t -s [Address]: Source address of tunnel (local side) \n"
		   "\t -d [Address]: Destination address of tunnel (remote side) \n"
		   "\t    [Address format]=IP:PORT \n"); 
    exit(0); 
} 


// Set addresses and create socket 
void network_init() 
{
	int i;
	int optval = 1;
	char *tun_dst_ip;

	// App socket 
	fectun->app_sock = socket_init(&fectun->app_src_addr, &fectun->app_dst_addr);
	memset(&fectun->app_dst, 0, sizeof(struct sockaddr_in));
	fectun->app_dst.sin_family = AF_INET;
	fectun->app_dst.sin_addr.s_addr = inet_addr(fectun->app_dst_addr.ip); 
	fectun->app_dst.sin_port = htons(fectun->app_dst_addr.port);
	
	// Tunnel socket 
	for (i = 0; i < fectun->num_paths; i++)
	{
		fectun->tun_sock[i] = socket_init(&fectun->tun_src_addr[i], &fectun->tun_dst_addr[i]);
		memset(&fectun->tun_dst[i], 0, sizeof(struct sockaddr_in));
		fectun->tun_dst[i].sin_family = AF_INET;
		fectun->tun_dst[i].sin_addr.s_addr = inet_addr(fectun->tun_dst_addr[i].ip); 
		fectun->tun_dst[i].sin_port = htons(fectun->tun_dst_addr[i].port);

		fectun->total_sent_pkts[i] = 0;
		fectun->total_sent_bytes[i] = 0;
		fectun->throughput[i] = 0;
		fectun->start_time[i] = 0.0;
	}
}

int socket_init(addr_info_t *src_addr, addr_info_t *dst_addr) 
{
	struct sockaddr_in src, dst; 
	int sock;
	int optval = 1; 

	// source address 	
	if (src_addr != NULL) 
	{
		memset(&src, 0, sizeof(struct sockaddr_in));
		src.sin_family = AF_INET;
		src.sin_addr.s_addr = inet_addr(src_addr->ip); 
		src.sin_port = htons(src_addr->port);
	}
	
	// destination address
	if (dst_addr != NULL)
	{
		memset(&dst, 0, sizeof(struct sockaddr_in));
		dst.sin_family = AF_INET;
		dst.sin_addr.s_addr = inet_addr(src_addr->ip); 
		dst.sin_port = htons(src_addr->port);
	}

	// Create socket 
	sock = socket(PF_INET, SOCK_DGRAM, 0);
	if (sock == -1)
		error_handling("UDP socket creation error");

	// Set socket reuse option 
	setsockopt(sock, SOL_SOCKET, SO_REUSEADDR, &optval, sizeof(optval));
	
	// Bind source IP address 
	if (bind(sock, (struct sockaddr*)&src, sizeof(struct sockaddr_in)) == -1)
	{
		print_log("port=%d \n", src_addr->port);
		error_handling("bind() error");
	}

	// Connect to destination 
	// if (connect(sock, (struct sockaddr*)&dst, sizeof(dst)) == -1)
	// {
	// 	error_handling("connect() error");
	// }

	// Timeout configuration
	struct timeval opttime = {SOCKET_TIMEOUT, 0};
	int optlen = sizeof(opttime);
	setsockopt(sock, SOL_SOCKET, SO_RCVTIMEO, &opttime, optlen);

	return sock;
}


void tunnel_start() 
{
	int ret; 
	int *idx = malloc(sizeof(int) * (fectun->num_paths + 1));
	pthread_t *tid = malloc(sizeof(pthread_t) * (fectun->num_paths + 1)); // including app side socket 

	// For upper layer
	idx[0] = UPPER_LAYER;
	ret = pthread_create(&tid[0], NULL, tunnel_recv_thread, (void*)&idx[0]);
	if (ret < 0)
		error_handling("pthread create error for upper layer");

	// For receiver mode, create receiver threads as many as num_paths 
	for (int i = 1; i < fectun->num_paths+1; i++)
	{
		idx[i] = i-1;
		ret = pthread_create(&tid[i], NULL, tunnel_recv_thread, (void*)&idx[i]);
		if (ret < 0)
			error_handling("pthread create error for lower layer");
	}

	for (int i = 0; i < fectun->num_paths + 1; i++) 
	{
		pthread_join(tid[i], NULL);
	}
}


void *tunnel_recv_thread(void *arg)
{
	int sock;
	int path = *((int*)arg);

	char *packet;
	uint32_t packet_len;
	uint32_t packet_seq, block_seq, symbol_seq, end_packet_seq;
	struct sockaddr_in recv_adr;
	socklen_t recv_adr_sz;	
	raptor_packet_t *received_pkt;

	print_log("[Path=%d] Recv Thread Started! ", path);

	if (path == UPPER_LAYER)
		sock = fectun->app_sock;
	else if (path == 0 || path == 1)
		sock = fectun->tun_sock[path]; 
	else 
		error_handling("Path error!!");

	packet_seq = 0; 
	while (1) 
	{
		packet = (char *) malloc(BUF_SIZE);		// Using pool???
		recv_adr_sz = sizeof(recv_adr);
		packet_len = recvfrom(sock, packet, BUF_SIZE, 0, (struct sockaddr*)&recv_adr, &recv_adr_sz);	
		if (packet_len == -1)
			error_handling("FECTUN: No activity! (Timeout)");

		//if (packet_seq % 1000 == 0)
			print_log("[Path=%d] Rx packet_seq=%d, len=%d, recv_port=%d", path, packet_seq, packet_len, ntohs(recv_adr.sin_port));
	
		if (path == UPPER_LAYER) 		// Packet from upper layer 
		{
			if (!fectun->fec_mode || (fectun->fec_mode && fectun->receiver_mode))  
			{
				// Send packet 
				send_raptor_packet(SOURCE_SYMBOL, packet_seq, 0, 0, 0, 0, packet, packet_len);
				packet_seq++; 
			}
			else if (fectun->fec_mode && !fectun->receiver_mode)
			{
				// Push source data from upper layer to raptor encoder 
				raptor_push_to_encoder(packet, packet_len, &packet_seq, &block_seq, &symbol_seq, &end_packet_seq);

				// make raptor_packet for source data and send it
				send_raptor_packet(SOURCE_SYMBOL, packet_seq, block_seq, symbol_seq, 0, end_packet_seq, packet, packet_len);
			}
			else 
			{
				error_handling("Error: Packet delivered from upper layer!");
			}							
		} 
		else 	// Packet from lower layer 
		{
			received_pkt = (raptor_packet_t *)packet;
			recv_raptor_packet(received_pkt);

			if (!fectun->fec_mode || (fectun->fec_mode && !fectun->receiver_mode)) 
			{
				// Send raw packet to upper layer
				sendto(fectun->app_sock, received_pkt->payload, received_pkt->len, 0, 
						(struct sockaddr*)&fectun->app_dst, sizeof(struct sockaddr_in));
			}
			else if (fectun->fec_mode && fectun->receiver_mode)
			{
				// Push to raptor modeule 
				raptor_push_to_decoder(path, received_pkt);
			}
			else 
			{
				error_handling("Error2: Packet delivered from upper layer!");
			}

			// if (packet_seq % 100 == 0)
				// printf("[RX] recv_addr_port=%d / packet len=%d / ingress=%d \n", recv_port, packet_len, packet_seq); 								

			packet_seq++;
		}	
	}	

	close(sock);
}


// Transmit redundant data to 
void *send_encoded_data() 
{
	// Get FEC encoded data from encode_blocks 
	raptor_block_t *block = raptor_get_encoded_data();

	if (block != NULL && block->data != NULL) 
	{
		int i; 
		int num_pushed_symbols = block->num_source_symbols + block->num_pushed_redundant_symbols; 
		int num_pkt = num_pushed_symbols / block->symbol_size; 

		if (block->block_seq % 1000 == 0)
		{
			print_log("TX Redundant: block_seq=%d, num_actual_source_symbols=%d, range=%d-%d", 
						block->block_seq, block->num_actual_source_symbols, block->start_packet_seq, block->end_packet_seq);
		}
		
		for (i = block->num_source_symbols; i < block->num_total_symbols; i++) 
		{		
			send_raptor_packet(REDUNDANT_SYMBOL, 0, block->block_seq, i, 
								block->start_packet_seq, block->end_packet_seq, 
								block->data[i], block->symbol_size);
		}

		free(block);
	}	
}


void *send_decoded_data(raptor_packet_t *pkt) 
{
	// transmit to upper layer 
	if (pkt->len > PAYLOAD_SIZE || pkt->len <= 0) {
		print_log("pkt->len=%d ", pkt->len);
	}

	sendto(fectun->app_sock, pkt->payload, pkt->len, 0, (struct sockaddr*)&fectun->app_dst, sizeof(struct sockaddr_in));	
	// send(fectun->app_sock, pkt->payload, pkt->len, 0);
}

// Transmit raptor packet with source
void send_raptor_packet(uint32_t type, uint32_t packet_seq, uint32_t block_seq, uint32_t symbol_seq, 
						uint32_t start_seq, uint32_t end_seq, char *data, uint32_t len)
{
	static int tx_path = 0;
	double cur_time = 0.0;

	// Make a FEC packet 
	raptor_packet_t *raptor_pkt = (raptor_packet_t *) malloc(sizeof(raptor_packet_t));	// TODO: Optimization -> define as global variable? pool? 
	raptor_pkt->type = htonl(type); 
	raptor_pkt->len = htonl(len); 
	raptor_pkt->packet_seq = htonl(packet_seq); 
	raptor_pkt->block_seq = htonl(block_seq); 
	raptor_pkt->symbol_seq = htonl(symbol_seq);
	raptor_pkt->start_packet_seq = htonl(start_seq); 
	raptor_pkt->end_packet_seq = htonl(end_seq); 

	memcpy(raptor_pkt->payload, data, len);		// Can we make it more efficiently? 
	
	// Transmit packet according to determined schedule 
	tx_path = packet_scheduling();

	// Transmit encoded data to remote side (destination)
	sendto(fectun->tun_sock[tx_path], raptor_pkt, RAPTOR_PACKET_HDR_LEN + len, 0, 
			(struct sockaddr*)&fectun->tun_dst[tx_path], sizeof(struct sockaddr_in));	
	// send(fectun->tun_sock[tx_path], raptor_pkt, RAPTOR_PACKET_HDR_LEN + len, 0);
	
	// After transmit, update measurement 
	update_measurement(tx_path, len); 

	if (raptor_pkt->packet_seq % 1000 == 0) 
	{
		print_log("Path[%d]: Tx Raptor Packet: type=%d, block_seq=%d, packet_seq=%d, symbol_seq=%d, len=%d, TxBytes=%d, Bucket=%d:%d, Tput=%.4f:%.4f", 
					tx_path, type, block_seq, packet_seq, symbol_seq, len, 
					fectun->total_sent_bytes[tx_path], fectun->pkt_bucket[0], fectun->pkt_bucket[1], 
					(double)(fectun->throughput[0])/(1024.0*1024.0), (double)(fectun->throughput[1])/(1024.0*1024.0));
	}
}

void recv_raptor_packet(raptor_packet_t *packet)
{
	// Convert field byte order to host byte order 
	packet->type = ntohl(packet->type);
	packet->len = ntohl(packet->len);
	packet->packet_seq = ntohl(packet->packet_seq);
	packet->block_seq = ntohl(packet->block_seq);
	packet->symbol_seq = ntohl(packet->symbol_seq);
	packet->start_packet_seq = ntohl(packet->start_packet_seq);
	packet->end_packet_seq = ntohl(packet->end_packet_seq);
}


int packet_scheduling() 
{
	int tx_path = 0;
	int sec_path = fectun->secondary_path; 
	if (fectun->num_paths == 1 || sec_path <= 0) 
	{
		// When single path configuration or secondary path is not active 
		if (fectun->pkt_bucket[0] == 0)
		{
			fectun->pkt_bucket[0] = TOTAL_BUCKET;
			fectun->pkt_bucket[1] = 0;
			fectun->throughput[1] = 0;
			
			print_log("packet_scheduling(): Bucket=%d:%d, Tput=%d:%d", 
						fectun->pkt_bucket[0], fectun->pkt_bucket[1], 
						fectun->throughput[0], fectun->throughput[1]);
		}

		// Send via primary path  
		tx_path = 0;
	}
	else 
	{
		// When secondary path is active 

		// Determine packet bucket according to measured throughput 
		if (fectun->pkt_bucket[0] == 0 && 
			fectun->pkt_bucket[sec_path] == 0)
		{
			// Set the throughput of secondary path to that of primary path to evenly distribute when initial stage 
			double cur_time = get_timestamp();
			if (fectun->start_time[sec_path] == 0.0 || cur_time - fectun->start_time[sec_path] <= 0.2)
				fectun->throughput[sec_path] = fectun->throughput[0];
			
			double pri_bucket = ((double)TOTAL_BUCKET * (double)fectun->throughput[0]) / 
								((double)fectun->throughput[0] + (double)fectun->throughput[sec_path]);
			fectun->pkt_bucket[0] = (int) ceil(pri_bucket); 
			fectun->pkt_bucket[sec_path] = min(TOTAL_BUCKET, max(TOTAL_BUCKET - fectun->pkt_bucket[0], 0));
			
			print_log("packet_scheduling(): Bucket=%d:%d, Tput=%d:%d, pri_bucket=%f", 
						fectun->pkt_bucket[0], fectun->pkt_bucket[sec_path], 
						fectun->throughput[0], fectun->throughput[sec_path], pri_bucket);
		}

		if (fectun->pkt_bucket[0] > 0)
		{
			// Send via primary path  
			tx_path = 0;
		}
		else 
		{
			// Send via secondary path  
			tx_path = sec_path; 
		}
	}

	// Decrease packet bucket 
	fectun->pkt_bucket[tx_path]--;
	if (fectun->pkt_bucket[tx_path] < 0)
		fectun->pkt_bucket[tx_path] = 0;

	// Perform path selection for next round when every bucket become empty 
	if ((tx_path == 0 && sec_path == 0 && fectun->pkt_bucket[0] == 0) ||
	    (tx_path == sec_path && fectun->pkt_bucket[0] == 0 && fectun->pkt_bucket[sec_path]))
	{
		path_selection();
	}

	return tx_path; 
}


void path_selection()
{	
	static double prev_time = 0.0; 
	static int prev_throughput = 0; 
	static int max_throughput[MAX_NUM_PATH] = {0};
	static int finish = 0;

	if (fectun->num_paths > 1 && fectun->secondary_path != -1 && finish == 0) 
	{	
		double cur_time = get_timestamp();
		
		if (prev_time == 0.0)
		{
			prev_time = cur_time; 
			return; 
		}

		if (cur_time - prev_time > TIME_WINDOW)
		{
			int cur_throughput = fectun->throughput[0];  
			if (fectun->secondary_path > 0)
				cur_throughput += fectun->throughput[fectun->secondary_path];

			if (prev_throughput == 0)
			{
				prev_time = cur_time; 
				prev_throughput = cur_throughput;
				return;
			}
			
			if (abs(cur_throughput - prev_throughput) <= THROUGHPUT_WINDOW)
			{
				// When throughput doesn't change

				// Reset secondary path
				if (fectun->secondary_path != 0)
				{
					fectun->throughput[fectun->secondary_path] = 0;
					fectun->pkt_bucket[fectun->secondary_path] = 0;
				}

				if (fectun->secondary_path != fectun->num_paths - 1)
				{
					// fectun->secondary_path is not last path 

					// Set max throughput 
					max_throughput[fectun->secondary_path] = cur_throughput;

					// Change to next secondary path 
					fectun->secondary_path++;
				}
				else 
				{
					// fectun->secondary_path is last path -> examined all paths

					// Check maximum throughput 
					int max = max_throughput[0];
					int max_path = 0; 
					max_throughput[fectun->secondary_path] = cur_throughput;

					for (int i = 1; i < fectun->num_paths; i++)
					{
						if (max < max_throughput[i] && abs(max - max_throughput[i]) > THROUGHPUT_WINDOW)
						{
							max = max_throughput[i];
							max_path = i; 
						}
					}
					
					if (max_path == 0)
						fectun->secondary_path = -1;
					else 
						fectun->secondary_path = max_path; 

					finish = 1;
				}
				
				//fectun->start_time[fectun->secondary_path] = cur_time; 
				print_log("Path[%d]: Secondary Path is %d (cur_throughput=%d, prev_throughput=%d, finish=%d, max=%d:%d)", 
							fectun->secondary_path, fectun->secondary_path, cur_throughput, prev_throughput, finish, max_throughput[0], max_throughput[1]);
			}
			
			prev_time = cur_time;
			prev_throughput = cur_throughput;
		}
	}
}


void update_measurement(int path, uint32_t sent_bytes)
{
	// Measure a throughput 
	double cur_time = get_timestamp(); 
	if (fectun->start_time[path] == 0.0 || fectun->start_time[path]-cur_time >= 2.0) 
	{
		fectun->total_sent_pkts[path] = 0;
		fectun->total_sent_bytes[path] = 0;
		fectun->start_time[path] = cur_time;		
	}

	fectun->total_sent_pkts[path]++;
	fectun->total_sent_bytes[path] += sent_bytes;
	fectun->throughput[path] = (int)(0.8 * fectun->throughput[path]) + (int) (0.2 * (fectun->total_sent_bytes[path] * 8.0) / (cur_time - fectun->start_time[path]));
}


void error_handling(char *message)
{
	fputs(message, stderr);
	fputc('\n', stderr);
	exit(1);
}