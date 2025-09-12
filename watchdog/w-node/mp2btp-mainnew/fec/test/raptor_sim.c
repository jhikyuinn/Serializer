/* raptor_measure.c */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/time.h>
#include <math.h>

#include "raptor.h"

void get_options(int argc, char *argv[]);
void raptor_simulation(); 
char **encoding(struct RaptorParams *params, char **src);
char **decoding(struct RaptorParams *params, char **enc, int *index);
int *simulate_symbol_loss();
char **make_lossy_matrix(char **enc, int *index);
int compare_2d_arrays(char **a, char **b, int row, int col);
void read_file(char **src, int row, int col, FILE *fp);
void print_results();
void print_hex_bytes(const char *data, int length);


// Global variables (Options)
int symbol_size = 0;
int num_src_symbols = 0;
int num_enc_symbols = 0;
int num_xor_enc = 0;
int num_xor_dec = 0;
double code_rate = 0.0;
double loss_rate = 0.0;
char file_name[256] = {0};
int iterations = 0;
long t_tot_enc, t_tot_dec;	// usec 


// Usage 
// $ 

// Main
int main(int argc, char *argv[]) 
{
	// Get options 
	get_options(argc, argv);

	// Start Raptor Encoding and Decoding 
	raptor_simulation(); 

	// Print Results 
	print_results();
}

// Get options 
void get_options(int argc, char *argv[])
{
	int opt;
	while ((opt = getopt(argc, argv, "s:k:c:l:f:i:")) != -1) 
	{
		switch (opt) 
		{
			case 's':
				symbol_size = atoi(optarg);
				break;
			case 'k':
				num_src_symbols = atoi(optarg);
				break;
			case 'c':
				code_rate = atof(optarg);
				break;
			case 'l':
				loss_rate = atof(optarg);
				break;				
			case 'f':
				strncpy(file_name, optarg, sizeof(file_name) - 1);
				break;
			case 'i':
				iterations = atoi(optarg);
				break;
			default:
				fprintf(stderr, "Usage: %s -s <symbol size> -k <num symbols> -c <code rate> -l <loss rate> -f <file name> -i <iterations>\n", argv[0]);
				exit(EXIT_FAILURE);
		}
	}

	num_enc_symbols = (int) ceil((double)num_src_symbols / (double)(1.0 - code_rate)); 

	// Print Options 
	printf("---------------------------------------------\n");
	printf("Symbol size: %d\n", symbol_size);
	printf("Number of source symbols: %d\n", num_src_symbols);
	printf("Number of encoding symbols: %d\n", num_enc_symbols);
	printf("Code rate: %.2f\n", code_rate);
	printf("Loss rate: %.2f %% \n", loss_rate*100.0);
	printf("File name: %s\n", file_name);
	printf("Iterations: %d\n", iterations);
	printf("---------------------------------------------\n");
}


void raptor_simulation()
{
	struct RaptorParams *params;
	FILE *fp; 

	// Raptor initialization 
	params = RaptorInitialization(num_src_symbols, symbol_size, code_rate, code_rate);	// RAPTOR_MIN_OVERHEAD[0], code_rate)
	
	// File open 
	fp = fopen(file_name, "rb");
    if (!fp) 
	{
        perror("fopen");
        exit(1);
    }

	t_tot_enc = 0; 
	t_tot_dec = 0;
	
	// Simulation 
	for (int i = 0; i < iterations; i++) 
	{
		// Read file 
		char **src = MallocCharTwoArray(num_src_symbols, symbol_size);
		read_file(src, num_src_symbols, symbol_size, fp);

		// Raptor Encoding 
		char **enc = encoding(params, src); 
		if (enc == NULL)	
		{
			fprintf(stderr, "Encoding Failure! (iter=%d) -------------------------------- \n", i);
			continue;
		}

		// Symbol loss simulation
		int *index = simulate_symbol_loss(); 
		char **erc = make_lossy_matrix(enc, index);		

		// Raptor Decoding
		char **dec = decoding(params, erc, index);
		if (dec == NULL)	
		{
			fprintf(stderr, "Decoding Failure! (iter=%d) -------------------------------- \n", i);
			continue;
		}

		// Compare 
		int idx = compare_2d_arrays(src, dec, num_src_symbols, symbol_size);
		if (idx >= 0)
		{			
			fprintf(stderr, "----------Source and Encoded Data are not matched! (iter=%d) \n", i);
			printf("Source array (symidx=%d): \n", idx);
			print_hex_bytes(src[idx], 64); //symbol_size*num_enc_symbols); //symbol_size);//64
			printf("Target array (symidx=%d): \n", idx);
			print_hex_bytes(erc[idx], 64); // symbol_size*num_enc_symbols); //symbol_size);//64
			exit(1); 
		}
		else 
		{
			// fprintf(stderr, "Matched! (iter=%d) ----------------------------------------- \n", i);
		}

		// Free arrays
		FreeCharTwoArray(src, num_src_symbols);
		FreeCharTwoArray(enc, num_enc_symbols);
		FreeCharTwoArray(erc, num_enc_symbols);
		FreeCharTwoArray(dec, num_src_symbols);
		free(index);
	}
}


// Raptor Encoding 
char **encoding(struct RaptorParams *params, char **src)
{
	char **enc; 
	long t_elp;

	struct timeval start, end;

	gettimeofday(&start, NULL);
	enc = RaptorEncoder2(src, params, num_enc_symbols);
	if (enc == NULL) 
	{
		fprintf(stderr, "Encoding Failure!!\n");
		exit(1);	
	}
	gettimeofday(&end, NULL);

	// micro seconds 
	t_elp = (end.tv_sec - start.tv_sec) * 1000000 + (end.tv_usec - start.tv_usec);
	t_tot_enc += t_elp;
	
	// xor
	num_xor_enc = params->XORENC;

	return enc;
}


// Raptor Decoding 
char **decoding(struct RaptorParams *params, char **enc, int *index)
{
	char **dec; 
	long t_elp; 
	struct timeval start, end;

	gettimeofday(&start, NULL);
	dec = RaptorDecoder(enc, index, params);
	if (dec == NULL) 
	{
		fprintf(stderr, "Decoding Failure!!\n");
		exit(1);	
	}
	gettimeofday(&end, NULL);

	// micro seconds  
	t_elp = (end.tv_sec - start.tv_sec) * 1000000 + (end.tv_usec - start.tv_usec);	
	t_tot_dec += t_elp;

	// xor
	num_xor_dec = params->XORDEC;

	return dec;
}

int *simulate_symbol_loss()
{
	int idx = 0;
	int *index = (int *)malloc(num_enc_symbols * sizeof(int));

	// Simulate symbol loss 
	for (int i = 0; i < num_enc_symbols; i++) 
	{
		if (rand() % 10000 < (int)(loss_rate * 10000)) 
		{
			//printf("-- Symbol %d is lost --\n", i);
			//index[idx] = -1; // Mark as lost
			//idx++;
			index[i] = -1; // Mark as lost
		}
		else 
		{
			index[i] = i; 
		}
		//idx++;
	}

	// while (idx < num_enc_symbols) 
	// {
	// 	index[idx] = -1; // Mark as lost
	// 	idx++;
	// }

	return index;
}


char **make_lossy_matrix(char **enc, int *index)
{
	char **erc = MallocCharTwoArray(num_enc_symbols, symbol_size);
	for (int i = 0; i < num_enc_symbols; i++) 
	{
		if (index[i] == -1)
		{
			// Fill with zeros or some other value
			memset(erc[i], 0, symbol_size);
		}
		else 
		{
			// Copy the original data
			memcpy(erc[i], enc[index[i]], symbol_size);
		}
	} 

	return erc; 
}


int compare_2d_arrays(char **arr1, char **arr2, int row, int col) 
{
    for (int i = 0; i < row; i++) 
	{
		// Compare each row of the two arrays 
		if (arr1[i] == NULL || arr2[i] == NULL) {
			fprintf(stderr, "Error: One of the arrays is NULL at row %d\n", i);
			return -1;
		}

		// Compare the contents of the two rows
		if (memcmp(arr1[i], arr2[i], col) != 0) {
            // Print out the first mismatch 
            for (int j = 0; j < col; j++) {
                if (arr1[i][j] != arr2[i][j]) {
                    fprintf(stderr, "Mismatch at row %d, col %d: arr1=0x%02X, arr2=0x%02X\n",
							i, j, (unsigned char)arr1[i][j], (unsigned char)arr2[i][j]);
					return i;
                }
            }
		}
    }
    return -1;  // If match 
}

// Read data from file 
void read_file(char **src, int row, int col, FILE *fp) 
{
    for (int i = 0; i < row; i++)
	{
		size_t n = fread(src[i], sizeof(char), col, fp);
		if (n < col) 
		{
			fseek(fp, 0, SEEK_SET);
		}
	}
}


// Print Results 
void print_results() 
{
	int enc_xored_bytes = symbol_size * num_xor_enc; 
	int dec_xored_bytes = symbol_size * num_xor_dec; 
	double raptor_enc_time = (t_tot_enc / 1000.0) / iterations; // milliseconds 
	double raptor_dec_time = (t_tot_dec / 1000.0) / iterations; // milliseconds

	printf("Simulation Success!\n");
	printf("--------------------------------------------------\n");
	printf("        S\t K\t EncK\t xor\t xorbytes\t time \n");
	printf("[ENC] : %d\t %d\t %d\t %d\t %9d\t %.2f \n", 
			symbol_size, num_src_symbols, num_enc_symbols, num_xor_enc, enc_xored_bytes, raptor_enc_time);
	printf("[DEC] : %d\t %d\t %d\t %d\t %9d\t %.2f \n", 
			symbol_size, num_src_symbols, num_enc_symbols, num_xor_dec, dec_xored_bytes, raptor_dec_time);
}

void print_hex_bytes(const char *data, int length) 
{
    for (int i = 0; i < length; i++) 
	{
        printf("%02X ", (unsigned char)data[i]); 
        if ((i + 1) % 32 == 0) 
		{
            printf("\n");
        }
    }

    if (length % 32 != 0) {
        printf("\n");  
    }
}