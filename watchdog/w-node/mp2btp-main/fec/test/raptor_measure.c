/* raptor_measure.c */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/time.h>
#include <math.h>

#include "raptor.h"

#define	NUM_ITERATION		500		// 50
#define ENERGY_SLEEP_TIME	10.0		// 10 sec
#define CODERATE_UNIT		0.1

#define LOG_RAPTOR_MEASURE_ENCODING	"./log_raptor_enc.dat"
#define LOG_RAPTOR_MEASURE_DECODING	"./log_raptor_dec.dat"

//int raptor_measure(int argc, char *argv[]);


int main(int argc, char *argv[]) 
{
	int i, j, c, p;
	int idx_start;
	int type; 
	int num_iter;
	int sym_size;
	int num_src_symbols;
	int enc_xored_bytes; 
	int dec_xored_bytes; 
	double coderate;
	double sleep_time;
	double system_start_time;
	double raptor_start_time;
	double raptor_elp_time; 
	double raptor_enc_time;
	double raptor_dec_time;
	double *minCR = RAPTOR_MIN_OVERHEAD; 

	int *k = RAPTOR_NUM_SOURCE_SYMBOLS;		
	int *s = RAPTOR_SYMBOL_SIZE;		
	int max_num_symbols;		
	int max_num_sym_size;	
	struct timeval start, end;
	long t_elp, t_tot_enc, t_tot_dec;	// usec 
	struct RaptorParams *params;

	char **src;  
	char **e;
	char **d;

	FILE *fp_enc, *fp_dec; 

	// Check input arguments
	if (argc != 2) 
	{
		printf("Usage: %s <Type: 0=time measure, 1=min overhead> \n", argv[0]);
		return -1; 
	}

	type = atoi(argv[1]);
	max_num_symbols = sizeof(RAPTOR_NUM_SOURCE_SYMBOLS) / sizeof(int);
	num_iter = NUM_ITERATION;

	if (type == 0) 
	{
		idx_start = 0;
		max_num_sym_size = sizeof(RAPTOR_SYMBOL_SIZE) / sizeof(int);
		sleep_time = 0;
		printf("Now we measure the raptor encoding/decoding time complxity ! \n");
	} 
	else if (type == 1)
	{
		char **VV = MallocCharTwoArray(512, 1);

		for(i = 0; i < 512; i++) 
			VV[i][0] = (char)i;
		
		max_num_sym_size = sizeof(RAPTOR_SYMBOL_SIZE) / sizeof(int);
		for(i = 0; i < max_num_sym_size; i++)
		{	
			for (j = 0; j < max_num_symbols; j++)
			{
				params = RaptorInitialization(k[j], s[i], 0.01, 0.3);	// 0.03, 0.3   // 0.05, 0.5
				FindMinOverhead(VV, 0.001, 2000, params);
				RaptorDeinitialization(params);
			}
		}
		
		FreeCharTwoArray(VV, 512);

		return 0; 
	}
	else 
	{
		printf("Usage: %s %s <Type: 0=time measure, 1=min overhead> \n", argv[0], argv[1]);
		return -1;
	}

	fp_enc = fopen(LOG_RAPTOR_MEASURE_ENCODING, "w");
	fp_dec = fopen(LOG_RAPTOR_MEASURE_DECODING, "w");

	usleep(sleep_time * 1000 * 1000);
	
	system_start_time = get_timestamp();

	// Encoding and decoding!!! 	
	for (i = idx_start; i < max_num_sym_size; i++)		// symbol size
	{	
		for (j = 0; j < max_num_symbols; j++)					// num of symbols
		{				
			//for (c = 1; c <= 5; c++)																			// code rate
			for (c = 1; c <= 1; c++)																			// code rate
			{
				coderate = CODERATE_UNIT * c;
				params = RaptorInitialization(k[j], s[i], 0.05, 1.0);
				//params = RaptorInitialization(k[j], s[i], coderate, coderate);
				src = MallocCharTwoArray(k[j], s[i]);

				t_tot_enc = 0;
				t_tot_dec = 0;
				
				// Raptor encoding 
				// repeat encoding when the encoding energy consumption is measured 
				num_src_symbols = (int) ceil((double)k[j] / (double)(1.0 - coderate)); 
				raptor_start_time = get_timestamp() - system_start_time;
				for (p = 0; p < num_iter;  p++) 
				{
					gettimeofday(&start, NULL);
					//e = RaptorEncoder(src, params);
					e = RaptorEncoder2(src, params, num_src_symbols);
					gettimeofday(&end, NULL);

					t_elp = (end.tv_sec - start.tv_sec) * 1000000 + (end.tv_usec - start.tv_usec);
					t_tot_enc += t_elp;
				}		
				raptor_elp_time = get_timestamp() - system_start_time;

				// SYMSIZE K XORENC ENCTIME (msec)
				enc_xored_bytes = s[i]*params->XORENC; 
				raptor_enc_time = (t_tot_enc/1000.0)/num_iter; 
				printf("[%.6f-%.6f][ENC] : %d %d %d %d %d %d %f %f \n", raptor_start_time, raptor_elp_time, s[i], k[j], num_src_symbols, num_src_symbols, params->XORENC, enc_xored_bytes, enc_xored_bytes/32.0, raptor_enc_time);
				fprintf(fp_enc,"%.6f %.6f %d %d %d %d %d %d %f %f \n", raptor_start_time, raptor_elp_time, s[i], k[j], num_src_symbols, num_src_symbols, params->XORENC, enc_xored_bytes, enc_xored_bytes/32.0, raptor_enc_time);

				// sleep 
				usleep(sleep_time * 1000 * 1000);

				// Packet loss simulation 
				int *ESIs = (int*)malloc(sizeof(int)*params->N);
				char **erc = MallocCharTwoArray(params->N, params->SYMBOL_SIZE);
				for (p = 0; p < params->N; p++) 
				{
					ESIs[p] = p; 
					memcpy(erc[p], e[p], params->SYMBOL_SIZE);
				}

				// Raptor decoding  
				// repeat decoding when the decoding energy consumption is measured 
				raptor_start_time = get_timestamp() - system_start_time;
				for (p = 0; p < num_iter;  p++) 
				{
					gettimeofday(&start, NULL);
					d = RaptorDecoder(erc, ESIs, params);
					gettimeofday(&end, NULL);

					t_elp = (end.tv_sec - start.tv_sec) * 1000000 + (end.tv_usec - start.tv_usec);	
					t_tot_dec += t_elp;
				}		
				raptor_elp_time = get_timestamp() - system_start_time;

				// SYMSIZE K XORDEC DECTIME
				dec_xored_bytes = s[i]*params->XORDEC; 
				raptor_dec_time = (t_tot_dec/1000.0)/num_iter; 
				printf("[%.6f-%.6f][DEC] : %d %d %d %d %d %d %f %f \n", raptor_start_time, raptor_elp_time, s[i], k[j], num_src_symbols, num_src_symbols, params->XORENC, dec_xored_bytes, dec_xored_bytes/32.0, raptor_dec_time);
				fprintf(fp_dec,"%.6f %.6f %d %d %d %d %d %d %f %f \n", raptor_start_time, raptor_elp_time, s[i], k[j], num_src_symbols, num_src_symbols, params->XORENC, dec_xored_bytes, dec_xored_bytes/32.0, raptor_dec_time);

				FreeCharTwoArray(e, num_src_symbols);
				FreeCharTwoArray(d, params->K);
				free(src);
				fflush(stdout);

				usleep(sleep_time * 1000 * 1000);

				RaptorDeinitialization(params);
			}
		}
	}

	fclose(fp_enc);
	fclose(fp_dec);

	return 0;
}

