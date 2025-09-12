#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <fcntl.h>
#include <math.h>
#include <sys/time.h>
#include <time.h>
#include <unistd.h>
#include <immintrin.h>	// AVX
#include <pthread.h>
#include "raptor_api.h"


// Raptor Initialization 
void raptor_init(void *(*encoding_handler)(), void *(*decoding_handler)()) 
{
    int i, j, idx;
    int s, k;
    int num_symbol_size = sizeof(RAPTOR_SYMBOL_SIZE) / sizeof(int); 
    int num_source_symbols = sizeof(RAPTOR_NUM_SOURCE_SYMBOLS) / sizeof(int);
    raptor_handler.params = (raptor_params_t **) malloc(sizeof(raptor_params_t *) * (num_symbol_size * num_source_symbols));
    
    // Raptor initialization 
    for (i = 0; i < num_symbol_size; i++)
    {
        for (j = 0; j < num_source_symbols; j++)
        {
            s = RAPTOR_SYMBOL_SIZE[i];
            k = RAPTOR_NUM_SOURCE_SYMBOLS[j]; 
            idx = i * num_symbol_size + j; 
            
            raptor_handler.params[idx] 
                = RaptorInitialization(k, s, RAPTOR_MIN_OVERHEAD[0], RAPTOR_MAX_CODERATE);
            if (raptor_handler.params[idx] != NULL) 
            {
                print_log("Raptor initialization success! (s=%d, k=%d)", s, k); 
            }
            else
            {
                print_log("Raptor initialization failed! (s=%d, k=%d)", s, k); 
                exit(0);
            }
        }
    }

    raptor_handler.queue_source_blocks = queue_init();
    raptor_handler.queue_encode_blocks = queue_init();
    raptor_handler.queue_receive_blocks = queue_init();

    raptor_handler.source_block_seq = -1;

    raptor_handler.expected_packet_seq = 0;
    raptor_handler.expected_block_seq = 0;
    raptor_handler.delivered_packet_seq = -1;
    raptor_handler.delivered_block_seq = -1;

    raptor_handler.sent_packet_seq = 0;
    raptor_handler.clear_packet_seq = BUFFER_CLEAR_SIZE;

    raptor_handler.enc_thread = 0;
    raptor_handler.dec_thread = 0;

    pthread_mutex_init(&raptor_handler.mutex, NULL);
    pthread_cond_init(&raptor_handler.enc_cond, NULL);
    pthread_cond_init(&raptor_handler.dec_cond, NULL);

    // init source_packet_buffer and redundant_packet_buffer
    raptor_handler.num_enc_symbols = (int) ceil((double)raptor_handler.params[0]->K / RAPTOR_MAX_CODERATE); 
    raptor_handler.num_redundant_symbols = raptor_handler.num_enc_symbols - raptor_handler.params[0]->K;

    for (i = 0; i < RECEIVE_BUFFER_SIZE; i++)
    {
        // Init recv_source_buffer
        raptor_handler.recv_source_buffer[i] = NULL;

        // Init recv_block_buffer
        raptor_handler.recv_block_buffer[i] 
                    = raptor_create_block(i, raptor_handler.params[0]->SYMBOL_SIZE,
                                          raptor_handler.params[0]->K, raptor_handler.num_enc_symbols); 
        raptor_handler.recv_block_buffer[i]->redundant_packet_buffer 
                    = (raptor_packet_t **)malloc(sizeof(raptor_packet_t*) * raptor_handler.num_redundant_symbols);
        for (j = 0; j < raptor_handler.num_redundant_symbols; j++)
            raptor_handler.recv_block_buffer[i]->redundant_packet_buffer[j] = NULL;
    }

    raptor_handler.encoding_handler = encoding_handler;
    raptor_handler.decoding_handler = decoding_handler;
}

raptor_block_t *raptor_create_block(int seq, int symbol_size, int num_source_symbols, int num_total_symbols) 
{   
    raptor_block_t *new_block = (raptor_block_t *) malloc(sizeof(raptor_block_t));
    new_block->block_seq = seq;
    new_block->symbol_size = symbol_size; 
    new_block->num_source_symbols = num_source_symbols;
    new_block->num_total_symbols = num_total_symbols;
    new_block->num_actual_source_symbols = 0;
    new_block->num_pushed_source_symbols = 0;
    new_block->num_pushed_redundant_symbols = 0;
    new_block->start_packet_seq = 0;
    new_block->end_packet_seq = 0;
    new_block->last_receive_time = 0;
    new_block->encodable = 0;
    new_block->decodable = 0; 
    new_block->symbol_map = (int *) malloc(sizeof(int) * num_total_symbols);   // TODO: Make a function that creates 1-D int array with zeros
    new_block->data = MallocCharTwoArray(num_total_symbols, symbol_size);   
      
    return new_block;
}

void raptor_clear_block_with_seq(raptor_block_t *block, int seq)
{
    int i;
    block->block_seq = seq;
    block->num_actual_source_symbols = 0;
    block->num_pushed_source_symbols = 0;
    block->num_pushed_redundant_symbols = 0;
    block->start_packet_seq = 0;
    block->end_packet_seq = 0;
    block->last_receive_time = 0;
    block->encodable = 0;
    block->decodable = 0; 
    memset(block->symbol_map, 0, sizeof(int) * block->num_total_symbols);
    ClearCharTwoArray(block->data, block->num_total_symbols, block->symbol_size);   

    for (i = 0; i < raptor_handler.num_redundant_symbols; i++)
    {
        if (block->redundant_packet_buffer[i] != NULL)
            free(block->redundant_packet_buffer[i]);
        block->redundant_packet_buffer[i] = NULL;
    }
}

// Clear recv_block_buffer periodically 
void raptor_clear_buffer()
{
    if (raptor_handler.delivered_packet_seq - (int)raptor_handler.clear_packet_seq > BUFFER_CLEAR_SIZE * 2)
    {
        int i; 
        uint32_t start_packet_seq = raptor_handler.clear_packet_seq - BUFFER_CLEAR_SIZE;
        uint32_t end_packet_seq = raptor_handler.clear_packet_seq - 1;
        uint32_t start_block_seq = 0; 
        uint32_t end_block_seq; 
        raptor_packet_t *clear_pkt; 

        for (i = start_packet_seq; i <= end_packet_seq; i++)
        {
            clear_pkt = raptor_handler.recv_source_buffer[i % RECEIVE_BUFFER_SIZE];
            if (clear_pkt != NULL) 
            {
                if (start_block_seq == 0)
                {
                    start_block_seq = raptor_handler.recv_source_buffer[i % RECEIVE_BUFFER_SIZE]->block_seq;
                }
               
                end_block_seq = raptor_handler.recv_source_buffer[i % RECEIVE_BUFFER_SIZE]->block_seq;
                //print_log("clear pkt: pkt_seq=%d, block_seq=%d", clear_pkt->packet_seq, clear_pkt->block_seq);
                free(clear_pkt); 
            }
            raptor_handler.recv_source_buffer[i % RECEIVE_BUFFER_SIZE] = NULL;
        }

        for (i = start_block_seq; i <= end_block_seq; i++)
        {
            raptor_clear_block_with_seq(raptor_handler.recv_block_buffer[i % RECEIVE_BUFFER_SIZE], i + RECEIVE_BUFFER_SIZE); 
        }
        
        // print_log("Clear recv_source_buffer: %d-%d(%d-%d) (delv_packet=%d) / recv_block_buffer: %d-%d(%d-%d)", 
        //             start_packet_seq, end_packet_seq, start_packet_seq%RECEIVE_BUFFER_SIZE, end_packet_seq%RECEIVE_BUFFER_SIZE, raptor_handler.delivered_packet_seq,
        //             start_block_seq, end_block_seq, start_block_seq%RECEIVE_BUFFER_SIZE, end_block_seq%RECEIVE_BUFFER_SIZE, raptor_handler.delivered_block_seq);
        raptor_handler.clear_packet_seq += BUFFER_CLEAR_SIZE;
    }
}


// For sender
void raptor_push_to_encoder(char *source, uint32_t source_len, uint32_t *packet_seq, uint32_t *block_seq, uint32_t *symbol_seq, uint32_t *end_packet_seq) 
{
    raptor_block_t *cur_block = queue_rear(raptor_handler.queue_source_blocks); 
    raptor_block_t *next_block = NULL;

    // If cur_block is full already, we should create a new block
    if (cur_block != NULL && 
        cur_block->num_pushed_source_symbols == cur_block->num_source_symbols)
    {
        cur_block = NULL;
    }

    // create block if list is empty 
    if (cur_block == NULL) 
    {
        cur_block = raptor_create_block(++raptor_handler.source_block_seq, 
                                        raptor_handler.params[0]->SYMBOL_SIZE, raptor_handler.params[0]->K, 
                                        raptor_handler.params[0]->K); 
        cur_block->start_packet_seq = raptor_handler.sent_packet_seq;
        queue_push(raptor_handler.queue_source_blocks, cur_block); 
    }

    int remained_num_symbols = cur_block->num_source_symbols - cur_block->num_pushed_source_symbols;
    int required_num_symbols = (int)ceil((double)source_len / (double)cur_block->symbol_size);
    if (remained_num_symbols >= required_num_symbols)
    {   
        // Set packet seq, block seq, and symbol seq for packet transmission
        *packet_seq = raptor_handler.sent_packet_seq;
        *block_seq = cur_block->block_seq;
        *symbol_seq = cur_block->num_pushed_source_symbols;
        *end_packet_seq = 0;

        // push source symbols into source block
        raptor_push_symbol(cur_block, SOURCE_SYMBOL, cur_block->num_pushed_source_symbols, 
                           source, source_len);  
        cur_block->num_actual_source_symbols = cur_block->num_pushed_source_symbols;
    }
    else 
    {
        // Create next block if remained bytes in cur_block is not sufficient to store source data
        cur_block->num_pushed_source_symbols = cur_block->num_source_symbols;

        next_block = raptor_create_block(++raptor_handler.source_block_seq, 
                                         raptor_handler.params[0]->SYMBOL_SIZE, raptor_handler.params[0]->K, 
                                         raptor_handler.params[0]->K);                                                  
        next_block->start_packet_seq = raptor_handler.sent_packet_seq;                                                         
        queue_push(raptor_handler.queue_source_blocks, next_block);                                         

        // Set packet seq, block seq, and symbol seq for packet transmission
        *packet_seq = raptor_handler.sent_packet_seq;
        *block_seq = next_block->block_seq;   
        *symbol_seq = next_block->num_pushed_source_symbols; 
        *end_packet_seq = 0;

        // push source symbols into source block (next block)
        raptor_push_symbol(next_block, SOURCE_SYMBOL, next_block->num_pushed_source_symbols,
                           source, source_len);
        next_block->num_actual_source_symbols = next_block->num_pushed_source_symbols;                             
    }


    // Moved
    if (next_block == NULL)
        cur_block->end_packet_seq = raptor_handler.sent_packet_seq;
    else 
        cur_block->end_packet_seq = raptor_handler.sent_packet_seq - 1;

    // check encoding and perform encoding (thread)
    if (cur_block->num_pushed_source_symbols == cur_block->num_source_symbols) // Encoding Case #1: source block is full
    {   
        // if (next_block == NULL)
        //     cur_block->end_packet_seq = raptor_handler.sent_packet_seq;
        // else 
        //     cur_block->end_packet_seq = raptor_handler.sent_packet_seq - 1;

        cur_block->encodable = 1;
        *end_packet_seq = cur_block->end_packet_seq;
        // queue_push(raptor_handler.queue_source_blocks, cur_block); 

        if (cur_block->block_seq % 1000 == 0)
        {
            print_log("Raptor Encoding: block_seq=%d, num_pushed_source_symbols=%d, range=%d-%d", 
                    cur_block->block_seq, cur_block->num_pushed_source_symbols, 
                    cur_block->start_packet_seq, cur_block->end_packet_seq);
        }
    }

    // Start the raptor encoding thread 
    if (raptor_handler.enc_thread == 0)
    {
        pthread_create(&raptor_handler.enc_thread, NULL, (void*)raptor_encoding, NULL);            
    }

    // Increase sequence number 
    raptor_handler.sent_packet_seq++;
}

// For receiver
void raptor_push_to_decoder(int path, raptor_packet_t *packet) 
{
    // For source symbols 
    if (packet->type == SOURCE_SYMBOL)
    {
        raptor_push_to_decoder_source(path, packet);
    }
    // For redundant symbols 
    else if (packet->type == REDUNDANT_SYMBOL)
    {
        raptor_push_to_decoder_redundant(path, packet);
    }
}

// Push source symbols
void raptor_push_to_decoder_source(int path, raptor_packet_t *packet) 
{
    pthread_mutex_lock(&raptor_handler.mutex);   
    // Update recent_block_seq 
    if (packet->block_seq > raptor_handler.recent_block_seq)
        raptor_handler.recent_block_seq = packet->block_seq;

    // When the received packet is duplicated 
    if (packet->packet_seq < raptor_handler.expected_packet_seq) 
    {
        print_log("Path[%d]: Error: Duplicated packet (packet_seq=%d, exp_packet_seq=%d)", 
                    path, packet->packet_seq, raptor_handler.expected_packet_seq);
    }
    else 
    {
        // When the receive buffer size is too small 
        if ((packet->packet_seq - raptor_handler.expected_packet_seq) >= RECEIVE_BUFFER_SIZE)
        {
            print_log("Path[%d]: Error: Receive buffer size is too small (packet_seq=%d, exp_packet_seq=%d)", 
                    path, packet->packet_seq, raptor_handler.expected_packet_seq);
            exit(1);
        }

        // Push packet into the recv_source_buffer
        raptor_handler.recv_source_buffer[packet->packet_seq % RECEIVE_BUFFER_SIZE] = packet;
        // print_log("Path[%d]: RX Source: packet_seq=%d, block_seq=%d, symbol_seq=%d (exp_packet=%d, exp_block=%d)", 
        //                 path, packet->packet_seq, packet->block_seq, packet->symbol_seq, raptor_handler.expected_packet_seq, raptor_handler.expected_block_seq);     
        
        // When the expected packet is received 
        if (packet->packet_seq == raptor_handler.expected_packet_seq) 
        {
            // print_log("Path[%d]: RX Source: packet_seq=%d, block_seq=%d, symbol_seq=%d (exp_packet=%d, exp_block=%d)", 
            //             path, packet->packet_seq, packet->block_seq, packet->symbol_seq, raptor_handler.expected_packet_seq, raptor_handler.expected_block_seq);     

            // Update the expected sequence number 
            while (raptor_handler.recv_source_buffer[raptor_handler.expected_packet_seq % RECEIVE_BUFFER_SIZE] != NULL)
            {
                // Increase the expected packet sequence number
                raptor_handler.expected_packet_seq++; 

                // Increase the expected block sequence number
                if (packet->block_seq == raptor_handler.expected_block_seq + 1) 
                    raptor_handler.expected_block_seq++;

                // print_log("Path[%d]: RX Source: Udpate Expected Seq! packet=%d, exp_packet=%d, block=%d, exp_block=%d", 
                //         path, packet->packet_seq, raptor_handler.expected_packet_seq, packet->block_seq,  raptor_handler.expected_block_seq);    
            }
            
        }
        else // if (packet->packet_seq > raptor_handler.expected_packet_seq)
        {
            // Out-of-order packet receiving 
            // print_log("Path[%d]: RX Source: Error!(Out-of-order) packet=%d (exp_packet=%d), block_seq=%d(exp_block=%d), symbol_seq=%d, delv_block=%d", 
            //             path, packet->packet_seq,  raptor_handler.expected_packet_seq, packet->block_seq, raptor_handler.expected_block_seq, 
            //             packet->symbol_seq, raptor_handler.delivered_block_seq);

            // Reset block seq     
            int i, idx; 
            double cur_time = get_timestamp(); 
            for (i = raptor_handler.delivered_block_seq + 1; i <= packet->block_seq; i++)
            {
                idx = i % RECEIVE_BUFFER_SIZE; 
                if (raptor_handler.recv_block_buffer[idx]->last_receive_time == 0)
                    raptor_handler.recv_block_buffer[idx]->last_receive_time = cur_time;
            }
        }
    }
    pthread_mutex_unlock(&raptor_handler.mutex);
}


// Push redundant symbols 
void raptor_push_to_decoder_redundant(int path, raptor_packet_t *packet) 
{
    int i;
    raptor_block_t *cur_block;
    raptor_block_t *exp_block;
    //static raptor_block_t *prev_block = NULL;

    // Start decoding thread     
    if (raptor_handler.dec_thread == 0 && path == 0)
    {      
        raptor_handler.dec_thread = 1;

        pthread_attr_t attr;	
        size_t stacksize = 15 * 1024 * 1024;
        pthread_attr_init(&attr);
        // Set the stack size
        pthread_attr_setstacksize(&attr, stacksize);
        // Get the stack size 
        pthread_attr_getstacksize(&attr, &stacksize);
        print_log("Raptor Decoding Thread stack size = %zu bytes", stacksize);

        pthread_create(&raptor_handler.dec_thread, NULL, (void *)raptor_decoding, NULL);
    }

    // When the block for the received redundant symbol has been delivered to upper layer already 
    if (packet->block_seq < raptor_handler.expected_block_seq) 
    {
        // print_log("RX Redundant: block_seq=%d, symbol_seq=%d, range=%d-%d (exp_packet=%d, exp_block=%d) -> Drop#1", 
        //             packet->block_seq, packet->symbol_seq, packet->start_packet_seq, packet->end_packet_seq,
        //             raptor_handler.expected_packet_seq, raptor_handler.expected_block_seq);
        return; 
    }

    // Push received redundant packet into redundant_packet_buffer
    cur_block = raptor_handler.recv_block_buffer[packet->block_seq % RECEIVE_BUFFER_SIZE];
    if (cur_block != NULL) 
    {
        // Update block buffer information 
        cur_block->block_seq = packet->block_seq;
        cur_block->start_packet_seq = packet->start_packet_seq;
        cur_block->end_packet_seq = packet->end_packet_seq;
        cur_block->last_receive_time = get_timestamp();

        // print_log("RX Redundant: block_seq=%d, symbol_seq=%d, range=%d-%d (exp_packet=%d, exp_block=%d) ", 
        //         packet->block_seq, packet->symbol_seq, packet->start_packet_seq, packet->end_packet_seq,
        //         raptor_handler.expected_packet_seq, raptor_handler.expected_block_seq);

        // Push received redundant packet 
        int seq = packet->symbol_seq - cur_block->num_source_symbols;
        cur_block->redundant_packet_buffer[seq] = packet;

        // Check decodable status of expected block 
        if (packet->block_seq > raptor_handler.expected_block_seq)
        {
            exp_block = raptor_handler.recv_block_buffer[raptor_handler.expected_block_seq % RECEIVE_BUFFER_SIZE];
            if (exp_block != NULL && exp_block->decodable == 0)
            {
                exp_block->decodable = 1;   // No lost packets

                // Check lost packets in the exp_block 
                for (i = exp_block->start_packet_seq; i <= exp_block->end_packet_seq; i++)
                {
                    if (raptor_handler.recv_source_buffer[i % RECEIVE_BUFFER_SIZE] == NULL)
                    {
                        exp_block->decodable = 2;   // lost packet exists
                        break;
                    }
                }
            }

            // if (exp_block->decodable == 1)
            //     return; 
        }
    }

    // Update recent_block_seq 
    int prev_recent_block_seq = raptor_handler.recent_block_seq;
    if (packet->block_seq > raptor_handler.recent_block_seq)
        raptor_handler.recent_block_seq = packet->block_seq;

}


void raptor_push_to_recv_source_buffer(raptor_block_t *block, char **dec_data)
{
    int i, j; 
    int num; 
    raptor_packet_t *packet; 

    j = 0;
    for (i = block->start_packet_seq; i < block->end_packet_seq; i++)
    {
        packet = raptor_handler.recv_source_buffer[i%RECEIVE_BUFFER_SIZE]; 
        
        if (packet != NULL)
        {
            j += (int) ceil(packet->len / block->symbol_size); 
        }
        else    // push decoded packet to receive buffer
        {
            uint32_t packet_len, remained, written; 

            raptor_packet_t *new_packet
                    = (raptor_packet_t *) malloc(sizeof(raptor_packet_t));  // Fix me

            // Get packet length       
            memcpy(&packet_len, dec_data[j], sizeof(uint32_t)); 
            
            if (packet_len > 1500)
            {
                print_log("Actually Raptor Decoding Fail! raptor_push_to_recv_source_buffer(): Enter! packet_len=%d, i=%d, j=%d", packet_len, i, j); 
                raptor_handler.recv_source_buffer[i%RECEIVE_BUFFER_SIZE] = NULL;
                continue;
            }

            // Set new_packet 
            new_packet->type = SOURCE_SYMBOL;
            new_packet->packet_seq = i; 
            new_packet->block_seq = block->block_seq;
            new_packet->symbol_seq = j;
            new_packet->len = packet_len; 
        
			// Push decoded data into packet
            remained = packet_len;
			if (packet_len >= block->symbol_size - sizeof(uint32_t))
				written = block->symbol_size - sizeof(uint32_t); 
			else 
				written = packet_len; 

			memcpy(new_packet->payload, dec_data[j] + sizeof(uint32_t), written);
			remained -= written;   
			j++; 

			// Push remained decoded data into packet
			while (remained > 0 && j < block->num_source_symbols) 
			{
				if (remained >= block->symbol_size) 
				{
					memcpy(new_packet->payload + written, dec_data[j], block->symbol_size);
					remained -= block->symbol_size;
					written += block->symbol_size;
				}
				else 
				{
					memcpy(new_packet->payload + written, dec_data[j], remained);
					remained = 0; 
				}
				j++; 
			}

            raptor_handler.recv_source_buffer[i%RECEIVE_BUFFER_SIZE] = new_packet;
        }
    }
}


// For Receiver
int raptor_push_to_enc_block(raptor_block_t *block)
{
    int i, j; 
    int num_pushed_symbols; 
    int num_total_pushed_symbols = 0;
    raptor_packet_t *packet = NULL;

    block->num_pushed_source_symbols = 0;
    block->num_pushed_redundant_symbols = 0;
    //block->symbol_map = (int *)malloc(sizeof(int) * block->num_total_symbols);
    memset(block->symbol_map, 0, sizeof(int) * block->num_total_symbols);

    for (i = block->start_packet_seq; i <= block->end_packet_seq; i++)
    {
        packet = raptor_handler.recv_source_buffer[i%RECEIVE_BUFFER_SIZE]; 
        if (packet != NULL)
        {
            num_pushed_symbols = raptor_push_symbol(block, SOURCE_SYMBOL, num_total_pushed_symbols, 
                                                    packet->payload, packet->len);
            for (j = 0; j < num_pushed_symbols; j++)
            {
                block->symbol_map[num_total_pushed_symbols] = packet->symbol_seq + j;
                num_total_pushed_symbols++;
            }
        }
    }

    for (i = 0; i < block->num_total_symbols-block->num_source_symbols; i++)
    {
        packet = block->redundant_packet_buffer[i]; 
        if (packet != NULL)
        {
            num_pushed_symbols = raptor_push_symbol(block, REDUNDANT_SYMBOL, num_total_pushed_symbols, 
                                                    packet->payload, packet->len);
            for (j = 0; j < num_pushed_symbols; j++)
            {
                block->symbol_map[num_total_pushed_symbols] = packet->symbol_seq + j;
                num_total_pushed_symbols++;
            }
        }
    }

    return (block->num_total_symbols - num_total_pushed_symbols);
}


// Push symbols into block 
int raptor_push_symbol(raptor_block_t *block, uint32_t type, uint32_t seq, char *source, uint32_t source_len) 
{   
    uint32_t prev_symbol_seq = seq;
    uint32_t cur_symbol_seq = seq;  
    uint32_t remained = source_len; 
    uint32_t written = 0;
    
    if (type == SOURCE_SYMBOL)
    {
        // push length of source into first 4 bytes of symbol 
        memcpy(block->data[cur_symbol_seq], &source_len, sizeof(int)); 
        
        // push source data into remained bytes of current symbol
        if (source_len >= block->symbol_size - sizeof(int))
            written = block->symbol_size - sizeof(int); 
        else 
            written = source_len; 

        memcpy(block->data[cur_symbol_seq] + sizeof(int), source, written);
        remained -= written;   

        //block->receive_map[cur_symbol_seq] = 1;     // for receiver
        cur_symbol_seq++; 
    }

    // push remained source data into next symbol 
    while (remained > 0 && cur_symbol_seq < block->num_total_symbols) 
    {
        if (remained >= block->symbol_size) 
        {
            memcpy(block->data[cur_symbol_seq], source + written, block->symbol_size);
            remained -= block->symbol_size;
            written += block->symbol_size;
        }
        else 
        {
            memcpy(block->data[cur_symbol_seq], source + written, remained);
            remained = 0; 
        }
        //block->receive_map[cur_symbol_seq] = 1;     // for receiver
        cur_symbol_seq++; 
    }

    if (type == SOURCE_SYMBOL)
        block->num_pushed_source_symbols += (cur_symbol_seq - prev_symbol_seq);
    else if (type == REDUNDANT_SYMBOL)
        block->num_pushed_redundant_symbols += (cur_symbol_seq - prev_symbol_seq);

    block->last_push_symbol_time = get_timestamp();

    return (cur_symbol_seq - prev_symbol_seq);
}


// Raptor encoding process
void *raptor_encoding()
{
	int i, j;
	int num_enc_symbols;
	char **enc_data;
    double start_time, enc_time;
    raptor_block_t *block; 
    raptor_block_t *enc_block;
    print_log("Raptor Encoder Thread Start!"); 
    
    while (1)
    {
        //pthread_cond_wait(&raptor_handler.enc_cond, &raptor_handler.mutex);     // TODO: timed condition???
        //pthread_mutex_lock(&raptor_handler.mutex);
        //print_log("raptor_encoding()");
        block = queue_front(raptor_handler.queue_source_blocks);
        if (block == NULL)
            continue;

        if (block != NULL) 
        {
            // Check encoding timeout
            if (get_timestamp() - block->last_push_symbol_time >= ENCODING_TIMEOUT)
            {
                print_log("Encoding Timeout! block_seq=%d, num_pushed_syms=%d", 
                            block->block_seq, block->num_pushed_source_symbols);
                block->encodable = 1;
            }

            if (block->encodable == 0)
                continue; 
        }
        
        //     pthread_cond_wait(&raptor_handler.enc_cond, &raptor_handler.mutex); 
        //pthread_mutex_unlock(&raptor_handler.mutex);

        block = queue_pop(raptor_handler.queue_source_blocks);
        if (block == NULL)
            continue; 

        // print_log("Encoder start...");

        num_enc_symbols = (int) ceil((double)block->num_source_symbols / RAPTOR_MAX_CODERATE); 

        // print_hex_2d(block->data, block->num_total_symbols, 32);

        // Raptor encoding
        start_time = get_timestamp();
        enc_data = RaptorEncoder2(block->data, raptor_handler.params[0], num_enc_symbols);
        enc_time = get_timestamp() - start_time;
        if (enc_data == NULL)
        {
            print_log("Raptor encoding failed! %d", block->block_seq);
            exit(0);
        }
        else 
        {   
            // print_log("Raptor encoding success!: block_seq=%d, s=%d, k'=%d (enc_time=%.4fs)", 
            //             block->block_seq, block->symbol_size, num_enc_symbols, enc_time);
        }

        // Update block 
        FreeCharTwoArray(block->data, block->num_source_symbols);
        block->num_total_symbols = num_enc_symbols;

        // Tricky method -> To be modified 
        // if (block->block_seq <= 100)
        //     block->data = NULL;
        // else 
            block->data = enc_data;
        
        // Push block into queue_encode_blocks
        queue_push(raptor_handler.queue_encode_blocks, block); 
        //queue_push(raptor_handler.queue_encode_blocks, enc_block); 

        // Start callback function after Raptor encoding 
        raptor_handler.encoding_handler();

    }
}


// Raptor decoding process
void *raptor_decoding() 
{
    int prev_pkt_seq, prev_block_seq; 
    int num_lost_symbols;
    char **receive_block = NULL;
    char **dec_data = NULL;
    double start_time, dec_time;
    double last_receive_time, prev_last_receive_time;
    double cur_time;
    raptor_packet_t *pkt; 
    raptor_block_t *block;
    raptor_block_t *dec_block;
    raptor_params_t *params = raptor_handler.params[0];
    print_log("Raptor Decoder Thread Start!"); 

    while (1)
    {
        // Check the next of delivered_packet_seq when in-order packet delivery 
        pkt = raptor_handler.recv_source_buffer[(raptor_handler.delivered_packet_seq+1) % RECEIVE_BUFFER_SIZE];
        if (pkt != NULL)
        {
            // Send to upper layer 
            raptor_handler.decoding_handler(pkt);

            pthread_mutex_lock(&raptor_handler.mutex);

            // Update delivered_packet_seq 
            raptor_handler.delivered_packet_seq++; 

            // Update expected_packet_seq  
            if (raptor_handler.delivered_packet_seq == raptor_handler.expected_packet_seq)
            {
                // print_log("Push Source Packet: packet_seq=%d, block_seq=%d, exp_packet=%d, exp_block=%d",
                //             pkt->packet_seq, pkt->block_seq, raptor_handler.expected_packet_seq, raptor_handler.expected_block_seq);
                raptor_handler.expected_packet_seq++;
            }

            // Update delivered_block_seq 
            if (prev_pkt_seq + 1 == pkt->packet_seq && prev_block_seq + 1 == pkt->block_seq)
                raptor_handler.delivered_block_seq = prev_block_seq;  
            
            // Update expected_block_seq
            if (raptor_handler.delivered_block_seq == raptor_handler.expected_block_seq)
                raptor_handler.expected_block_seq++;

            // print_log("Push Source Packet: packet_seq=%d, block_seq=%d, symbol_seq=%d, end_packet_seq=%d (delv_packet=%d, delv_block=%d, exp_packet=%d, exp_block=%d)", 
            //             pkt->packet_seq, pkt->block_seq, pkt->symbol_seq, pkt->end_packet_seq,
            //             raptor_handler.delivered_packet_seq, raptor_handler.delivered_block_seq, 
            //             raptor_handler.expected_packet_seq, raptor_handler.expected_block_seq);    

            pthread_mutex_unlock(&raptor_handler.mutex);
            
            prev_pkt_seq = pkt->packet_seq; 
            prev_block_seq = pkt->block_seq;
        }
        
        // Check delivered_block_seq when out-of-order packet delivery 
        else 
        {
            block = raptor_handler.recv_block_buffer[(raptor_handler.delivered_block_seq+1) % RECEIVE_BUFFER_SIZE];

            // Calculate timer 
            cur_time = get_timestamp();
            if (block->last_receive_time > 0)
            {
                last_receive_time = block->last_receive_time;
                prev_last_receive_time = last_receive_time;
            }
            else    
            {
                last_receive_time = cur_time;
            }

            //print_log("Raptor Decoding Time: block_seq=%d, cur_time=%f, block_last_receive_time=%f", cur_time, block->last_receive_time);
            
            // Check whether the selected block is decodable or timeout 
            if (block->decodable > 1)   // Decodable=1 -> no lost 
            {
                // Push symbols to encoding block and get the number of lost symbols 
                num_lost_symbols = raptor_push_to_enc_block(block);

                print_log("Raptor Decoding: block_seq=%d, decodable=%d, range=%d-%d (delv_packet=%d, exp_packet=%d, exp_block=%d, tot_syms=%d, src_syms=%d)", 
                            block->block_seq, block->decodable, block->start_packet_seq, block->end_packet_seq, 
                            raptor_handler.delivered_packet_seq, raptor_handler.expected_packet_seq, raptor_handler.expected_block_seq, 
                            block->num_total_symbols, block->num_source_symbols); 

                dec_data = NULL;
                // Perform raptor decoding if the number of received symbols >= the number of source symbols 
                if (block->num_total_symbols - num_lost_symbols >= block->num_source_symbols)
                {
                    start_time = get_timestamp();
                    dec_data = RaptorDecoder(block->data, block->symbol_map, raptor_handler.params[0]);
                    dec_time = get_timestamp();
                }

                if (dec_data != NULL) 
                {   
                    print_log("Raptor Decoding Success!: block_seq=%d, range=%d-%d, num_pushed_source=%d, num_pushed_redundant=%d, num_lost_symbols=%d", 
                                block->block_seq, block->start_packet_seq, block->end_packet_seq, 
                                block->num_pushed_source_symbols, block->num_pushed_redundant_symbols, num_lost_symbols);

                    // Push to recv_source_buffer
                    raptor_push_to_recv_source_buffer(block, dec_data);
                }
                else 
                {
                    print_log("Raptor Decoding Fail!: block_seq=%d, range=%d-%d, num_pushed_source=%d, num_pushed_redundant=%d, num_lost_symbols=%d", 
                                block->block_seq, block->start_packet_seq, block->end_packet_seq, 
                                block->num_pushed_source_symbols, block->num_pushed_redundant_symbols, num_lost_symbols);
                }
            }
            else if (cur_time - last_receive_time > DECODING_WAIT_TIME && block->decodable == 0 && block->block_seq != 0)   // TODO: remove 'block->block_seq != 0'
            {
                block->decodable = 3;
                print_log("Raptor Decoding Timeout!: block_seq=%d, range=%d-%d, num_pushed_source=%d, num_pushed_redundant=%d", 
                        block->block_seq, block->start_packet_seq, block->end_packet_seq, 
                        block->num_pushed_source_symbols, block->num_pushed_redundant_symbols);
            }

            if (block->decodable > 0)
            {
                // Send packets in the selected block to upper layer 
                while ((raptor_handler.delivered_packet_seq+1) <= block->end_packet_seq)
                {
                    pkt = raptor_handler.recv_source_buffer[(raptor_handler.delivered_packet_seq+1) % RECEIVE_BUFFER_SIZE];
                    if (pkt != NULL)
                        raptor_handler.decoding_handler(pkt);   
                    
                    // Update delivered_packet_seq
                    raptor_handler.delivered_packet_seq++;
                }
                
                pthread_mutex_lock(&raptor_handler.mutex);

                // Update expected_packet_seq
                raptor_handler.expected_packet_seq = block->end_packet_seq + 1;

                // Update expected_block_seq 
                raptor_handler.delivered_block_seq = block->block_seq; 
                raptor_handler.expected_block_seq = block->block_seq + 1; 

                // Update expected_packet_seq
                if (raptor_handler.expected_packet_seq <= raptor_handler.delivered_packet_seq)      
                     raptor_handler.expected_packet_seq = raptor_handler.delivered_packet_seq + 1;
            
                // Update expected_block_seq 
                if (raptor_handler.expected_block_seq < raptor_handler.delivered_block_seq)
                     raptor_handler.expected_block_seq = raptor_handler.delivered_block_seq + 1;

                // print_log("Updated! delv_packet=%d, delv_block=%d, exp_packet_seq=%d, exp_block_seq=%d", 
                //             raptor_handler.delivered_packet_seq, raptor_handler.delivered_block_seq, 
                //             raptor_handler.expected_packet_seq, raptor_handler.expected_block_seq);

                pthread_mutex_unlock(&raptor_handler.mutex);
            }
        }

        // Clear buffer 
        raptor_clear_buffer();
    }
}


//
int raptor_find_block_from_queue(void *target, void *search)
{
    raptor_block_t *block = (raptor_block_t *)target; 
    int *seq = (int *)search; 

    // print_log("Find: block->block_seq=%d, *seq=%d", block->block_seq, *seq);
    if (block->block_seq == *seq)
        return 1;
    else 
        return 0; 
}


// Get encoded data from encode queue
raptor_block_t *raptor_get_encoded_data() 
{
    return queue_pop(raptor_handler.queue_encode_blocks); 
}

