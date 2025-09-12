#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <math.h>
#include <limits.h>
#include <time.h>

#define TRUE    1
#define FALSE   0

double scale = INT_MAX / 10000;
double c = 0.03;
double delta = 0.5;
int beta = 0;

int is_robust_soliton = 1;

int *degree;
//----- below maximum configurate is to save resources
//	protected final int MAX_SOURCE = 10000000;

//----- for test
// long seed_degree = 676L;
// long select_seed = 800L;

int *get_ideal_soliton(int num_source);
int *get_robust_soliton(int num_source);
int *get_pro(int num_source);
int *get_tau(int num_source);
int *get_pro2(int num_source);
int *get_degree(int num_source, int num_encoded, int seed);
int **get_matrix(int num_source, int num_encoded, int seed);
char *lt_encode(char *source, int num_source, int num_encoded, int sym_size, int seed);
char *lt_decode(char *encoded, int num_encoded, int num_source, int sym_size, char *non_error, int seed); //, int *success);
//int loss_simulation(char *encoded, int num_source, int num_encoded, int sym_size);
void print_matrix(const char *data, int len);
void print_int_matrix(const int *data, int len);


int *get_ideal_soliton(int num_source)
{
    int *rho = (int *) malloc(sizeof(int) * (num_source + 1));
    rho[1] = (int) (scale * (1.0 / num_source));
    for (int i = 2; i <= num_source; i++)
    {
        rho[i] = (int) (scale * (1.0 / (i * (i - 1.0))));
        rho[i] += rho[i - 1];
    }
    rho[0] = rho[num_source];
    return rho;
}

int *get_robust_soliton(int num_source)
{
    int *rho = get_ideal_soliton(num_source);
    int *tau = get_tau(num_source);
    int *mu = (int *) malloc(sizeof(int) * (num_source + 1));
    beta = rho[0] + tau[0];

    // printf("beta=%d (%f)\n", beta, beta / scale);

    // for (int i = 0; i <= num_source; i++)
    //     printf("rho[%d]=%d  /  tau[%d]=%d \n", i, rho[i], i, tau[i]);

    for (int i = 0; i <= num_source; i++)
    {
        mu[i] = (int) (scale * (rho[i] + tau[i]) / beta);
    }

    return mu;
}

int *get_pro(int num_source)
{
    if (is_robust_soliton)
        return get_robust_soliton(num_source);
    else
        return get_ideal_soliton(num_source);
}

int *get_tau(int num_source)
{
    int *tau = (int *) malloc(sizeof(int) * (num_source + 1));
    double R = c * log(num_source / delta) * sqrt(num_source);
    int t = (int) (num_source / R);
    // printf("k=%d, R=%f, t=%d \n", num_source, R, t);

    // When i = 1, ...,  k/R - 1
    for (int i = 1; i < t; i++)
    {
        tau[i] = (int) (scale * (R / num_source) * (1.0 / i));
        if (i != 1)
        {
            tau[i] += tau[i - 1];
        }
    }

    // When i = k/R
    tau[t] = tau[t - 1] + (int) (scale * (R * log(R / delta)) / num_source);

    // When i = k/R + 1, ..., k
    for (int i = t + 1; i <= num_source; i++)
    {
        tau[i] = tau[i-1];
    }

    // Last
    tau[0] = tau[num_source];

    return tau;
}

int *get_pro2(int num_source)
{
    int *pro = (int *) malloc(sizeof(int) * (num_source + 1));
    double scale = INT_MAX / 3;
    //--- int F = (int) Math.floor(Math.log(0.01 * 0.01 / 4) / Math.log(1 - 0.01 / 2));
    int F = 2114;
    //============================================= when degree is one =====
    pro[1] = (int) (scale * (1.0 - (1.0 + 1.0 / F) / (1.0 + 0.01)));
    //===================================== when degree is two or more =====
    for (int i = 2; i <= num_source; i++)
    {
        pro[i] = pro[i - 1] + (int) (scale * ((1 - pro[1] / scale) / ((1 - 1.0 / F) * i * (i - 1))));
    }
    pro[0] = pro[num_source];
    return pro;
}


int *get_degree(int num_source, int num_encoded, int seed)
{
    srand(seed);

    int *pro = get_pro2(num_source); // get_pro2
    // for (int i = 0; i <= num_source; i++)
    // {
    //     printf("pro[%d]: %d \n", i, pro[i]);
    // }

    degree = (int *) malloc(sizeof(int) * num_encoded);

    for (int i = 0; i < num_encoded; i++)
    {
        int tmp = rand() % pro[0];  // 확인 필요!
        for (int j = 1; j <= num_source; j++)
        {
            if (tmp < pro[j]) {
                degree[i] = j;
                break;
            }
        }
    }

    return degree;
}

int **get_matrix(int num_source, int num_encoded, int seed)
{
    degree = get_degree(num_source, num_encoded, seed);
    srand(seed);
    int **matrix = (int **) malloc(sizeof(int *) * num_encoded);
    int *prevent_duplicate = malloc(sizeof(int) * num_source);

    // printf("\n\nMatrix:\n");
    for (int i = 0; i < num_encoded; i++)
    {
        memset(prevent_duplicate, 0, sizeof(int) * num_source);
        matrix[i] = (int *) malloc(sizeof(int) * degree[i]);
        // printf("matrix[%d]: degree=%d: ", i, degree[i]);
        for (int j = 0; j < degree[i]; j++)
        {
            while (TRUE)
            {
                int temp = rand() % num_source;
                if (prevent_duplicate[temp] == FALSE)
                {
                    matrix[i][j] = temp;
                    prevent_duplicate[temp] = TRUE;
                    break;
                }
            }
            // printf("%d, ", matrix[i][j]);
        }
        // printf("\n");
    }
    return matrix;
}


char *lt_encode(char *source, int num_source, int num_encoded, int sym_size, int seed)
{
    int **matrix = get_matrix(num_source, num_encoded, seed);
    char *encoded = (char *) malloc(sizeof(char) * (num_encoded * sym_size));
    memset(encoded, 0, sizeof(char) * (num_encoded * sym_size));

    for (int i = 0; i < num_encoded; i++)
    {
        for (int j = 0; j < degree[i]; j++)
        {
            for (int k = 0; k < sym_size; k++)
            {
                encoded[(i * sym_size) + k] ^= source[(matrix[i][j]*sym_size) + k];
            }
        }
    }
    return encoded;
}



char *lt_decode(char *encoded, int num_encoded, int num_source, int sym_size, char *non_error, int seed)
{
    // the generator matrix
    int **matrix = get_matrix(num_source, num_encoded, seed);

    // the space in which the reconstructed symbols placed
    char *reconstructed = (char *) malloc(sizeof(char) * (num_source * sym_size));
    memset(reconstructed, 0, sizeof(char) * (num_source * sym_size));

    // the array which represent where the reconstruction is finished
    int *finished = (int *) malloc(sizeof(int) * num_source);
    for (int i = 0; i < num_source; i++)
        finished[i] = 0;

    // the copy of the degrees because it has to be modified on the decoding process
    int *local_degree = (int *) malloc(sizeof(int) * num_encoded);
    for (int i = 0; i < num_encoded; i++)
        local_degree[i] = degree[i];

    // the number of the reconstructed source symbols
    int count = 0;

    // a hundred is the number we arbitrary defined
    // this number can be modified for better protection or less complexity
    for (int k = 0; k < 1000; k++)
    {
        // to monitor where the process is stagnat or not
        int tmp_checker = 0;

        // process the encoded symbols with degree one
        for (int i = 0; i < num_encoded; i++)
        {
            // find out
            // the degree of the encoded symbol is one and
            // the encoded symbol is intactly arrived
            if (local_degree[i] == 1 && non_error[i])
            {
                // check all source symbols which related this encoded symbol
                // because we don't know which connection is still remaining
                for (int j = 0; j < degree[i]; j++)
                {
                    // find only one source symbol which is still connected
                    // if that source symbol is already reconstructed we skip this process
                    int sel = matrix[i][j];
                    if (sel != -1 && finished[sel]==0)
                    {
                        // reconstruct the source symbol
                        for (int q = 0; q < sym_size; q++)
                        {
                            reconstructed[(sel * sym_size) + q] = encoded[(i * sym_size) + q];
                        }
                        finished[sel] = 1;
                        //success[sel] = TRUE;

                        // mark that the connection is broken
                        matrix[i][j] = -1;
                        local_degree[i]--;
                        count++;
                        tmp_checker++;
                    }
                }
            }
        }

        // process the ripples
        for (int i = 0; i < num_encoded; i++)
        {
            if (non_error[i])
            {
                for (int j = 0; j < degree[i]; j++)
                {
                    int sel = matrix[i][j];
                    if (sel != -1 && finished[sel] == 1)
                    {
                        for (int q = 0; q < sym_size; q++)
                        {
                            encoded[(i * sym_size) + q] ^= reconstructed[(sel * sym_size) + q];
                        }
                        matrix[i][j] = -1;
                        local_degree[i]--;
                        tmp_checker++;
                    }
                }
            }
        }

        // to avoid unnecessary loop
        if (count >= num_source || tmp_checker == 0)
        {
            break;
        }
    }


    return reconstructed;
}


void print_matrix(const char *data, int len)
{
    int i;
    static int idx = 0;

    printf("Data %dth (len=%d):\n", idx++, len);
    printf("%05d: ", 0);
    for (i = 0; i < len; i++)
    {
        printf("%02x ", data[i] & 0xff);
        if ((i+1) % 32 == 0)
            printf("\n%05d: ", (i+1)/32);
    }
    printf("\n");

    fflush(stdout);
}


void print_int_matrix(const int *data, int len) {
    int i;
    static int idx = 0;

    printf("Int Matrix %dth (len=%d):\n", idx++, len);
    for (i = 0; i < 512; i++)
    {
        printf("%d ", data[i]);
        if ((i+1) % 32 == 0)
            printf("\n");
    }
    printf("\n");
}