CC 				= gcc
OBJ_DIR			= .
RAPTOR_DIR		= ../fec/raptor
RAPTOR_TEST_DIR = ../fec/test
DEBUG     		= -g #-pg -g #-fsanitize=address
CFLAGS    	= $(DEBUG) 
RAPTOR_CFLAGS 	= -pthread -lrt -lm -lpthread -I$(RAPTOR_DIR)
# RAPTOR_CFLAGS = -lm -I/usr/arm-linux-gnueabihf/include/ -Ofast -mcpu=cortex-a7 \
# 			-mtune=cortex-a7 -mfpu=neon -ffp-contract=fast -ftree-vectorize
# # RAPTOR_CFLAGS = -lm -I/usr/arm-linux-gnueabihf/include/ -Ofast -mcpu=cortex-a9 \
# 			-mtune=cortex-a9 -mfpu=neon -ffp-contract=fast -ftree-vectorize

all: util.o raptor.o queue.o raptor_api.o 
	$(CC) $(CFLAGS) -o fectun fectun.c $(OBJ_DIR)/util.o $(OBJ_DIR)/raptor.o $(OBJ_DIR)/queue.o $(OBJ_DIR)/raptor_api.o $(RAPTOR_CFLAGS)
#	$(CC) $(CFLAGS) -o raptor_measure $(RAPTOR_TEST_DIR)/raptor_measure.c $(OBJ_DIR)/util.o $(OBJ_DIR)/raptor.o $(RAPTOR_CFLAGS)
	
util.o :
	@ $(CC) $(CFLAGS) $(RAPTOR_DIR)/util.c -c -o $(OBJ_DIR)/util.o  $(RAPTOR_CFLAGS) 

raptor.o : 
	@ $(CC) $(CFLAGS) $(RAPTOR_DIR)/raptor.c -c -o $(OBJ_DIR)/raptor.o  $(RAPTOR_CFLAGS)

queue.o : 
	@ $(CC) $(CFLAGS) queue.c -c -o $(OBJ_DIR)/queue.o $(RAPTOR_CFLAGS)

raptor_api.o : 
	@ $(CC) $(CFLAGS) raptor_api.c -c -o $(OBJ_DIR)/raptor_api.o $(RAPTOR_CFLAGS)


clean:
	rm -f *.c~ fectun 
	rm $(OBJ_DIR)/*.o
