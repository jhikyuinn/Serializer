#ifndef RAPTOR_API_H
#define RAPTOR_API_H

#include <stdio.h> 
#include <stdlib.h> 
#include <stdint.h>
#include <unistd.h> 
#include <pthread.h>
#include <signal.h>
#include <time.h>
#include <sys/time.h>
#include "raptor.h"
#include "queue.h"

#define RAPTOR_PACKET_HDR_LEN   4 * 7   // 4 * 7 = 28 bytes
#define PAYLOAD_SIZE            1460    // To be verified whether 1460 is sufficient. 
#define SOURCE_SYMBOL           0
#define REDUNDANT_SYMBOL        1
#define DECODE_EXPIRE_TIME      100     // milliseconds
#define RECEIVE_BUFFER_SIZE     1000 
#define BUFFER_CLEAR_SIZE       RECEIVE_BUFFER_SIZE / 20     
#define DECODING_WAIT_TIME      0.2
#define ENCODING_TIMEOUT        0.05

#define NO_PACKET_LOSS          10      // using packet type

typedef struct raptor_packet {  // TODO: Field Optimization
    uint32_t type; 			        // Source symbol vs Redundant symbol?
    uint32_t packet_seq; 		    // Packet sequence number
    uint32_t block_seq; 		    // Block sequence number
    uint32_t symbol_seq;            // Symbol sequence number
    uint32_t start_packet_seq;      // Start packet sequence number in block
    uint32_t end_packet_seq;        // End packet sequence number in block    
    uint32_t len;                   // Length of payload
    char     payload[PAYLOAD_SIZE]; // Payload
} raptor_packet_t;

typedef struct raptor_block {
    uint32_t block_seq; 
    uint32_t symbol_size; 
    uint32_t num_source_symbols;             // number of source symbols in block
    uint32_t num_actual_source_symbols;      // number of actual source symbols in block
    uint32_t num_total_symbols;              // symbol_size * num_total_symbols = size of data (size of matrix)
    uint32_t num_pushed_source_symbols;      // number of pushed source symbols in block 
    uint32_t num_pushed_redundant_symbols;   // number of pushed redundant symbols in block

    uint32_t start_packet_seq;
    uint32_t end_packet_seq;

    double last_push_symbol_time;       // needed for encoding 
    double last_receive_time;           // needed for decoding 
    
    timer_t decode_timer;
    int encodable; 
    int decodable;
    int *symbol_map;
    char **data; 
    raptor_packet_t **redundant_packet_buffer;
} raptor_block_t; 


typedef struct raptor {
    // raptor parameters 
    raptor_params_t **params; 
   
    // Queues
    queue_t *queue_source_blocks;  
    queue_t *queue_encode_blocks; 
    queue_t *queue_receive_blocks;     

    int source_block_seq;
 
    uint32_t expected_packet_seq;
    uint32_t expected_block_seq;
    
    int delivered_packet_seq;
    int delivered_block_seq;

    uint32_t recent_block_seq;   
    uint32_t clear_packet_seq;
    uint32_t sent_packet_seq;

    pthread_t enc_thread;
    pthread_t dec_thread;
    pthread_cond_t enc_cond;
    pthread_cond_t dec_cond;
    pthread_mutex_t mutex;
    
    // callback function which is called after encoding process
    void *(*encoding_handler)(void); 

    // callback function which is called after decoding process
    void *(*decoding_handler)(raptor_packet_t *pkt);

    uint32_t num_enc_symbols;
    uint32_t num_redundant_symbols;

    raptor_packet_t *recv_source_buffer[RECEIVE_BUFFER_SIZE];   
    raptor_block_t *recv_block_buffer[RECEIVE_BUFFER_SIZE];   

    uint32_t stats_num_no_dec_blocks; 
    uint32_t stats_num_dec_fail_blocks; 
    uint32_t stats_num_dec_succ_blocks; 


} raptor_t;

static raptor_t raptor_handler; 

void raptor_init(void *(*cb_after_encoding)(), void *(*cb_after_decoding)());
raptor_block_t *raptor_create_block(int seq, int symbol_size, int num_source_symbols, int num_total_symbols);
void raptor_clear_block_with_seq(raptor_block_t *block, int seq);
void raptor_clear_buffer();
void raptor_push_to_encoder(char *source, uint32_t source_len, uint32_t *packet_seq, uint32_t *block_seq, uint32_t *symbol_seq, uint32_t *end_packet_seq);
void raptor_push_to_decoder(int path, raptor_packet_t *packet);
void raptor_push_to_decoder_source(int path, raptor_packet_t *packet);
void raptor_push_to_decoder_redundant(int path, raptor_packet_t *packet);
int  raptor_push_to_enc_block(raptor_block_t *block);
int raptor_push_symbol(raptor_block_t *block, uint32_t type, uint32_t seq, char *source, uint32_t source_len);
void raptor_push_to_recv_source_buffer(raptor_block_t *block, char **dec_data);
void *raptor_encoding();
void *raptor_decoding();
int raptor_find_block_from_queue(void *target, void *search);
int raptor_check_decodable(raptor_block_t *cur_block);
raptor_block_t *raptor_get_encoded_data();
raptor_packet_t *raptor_get_received_packet();


#endif 