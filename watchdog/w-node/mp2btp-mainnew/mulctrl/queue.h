#ifndef QUEUE_H
#define QUEUE_H

#include <stdio.h>
#include <stdlib.h>
#include <pthread.h>

typedef struct queue_node {
    void *data;
    struct queue_node *next;
} queue_node_t;

typedef struct queue {
    int size; 
    pthread_mutex_t mutex;
    queue_node_t *head;
    queue_node_t *tail;
} queue_t; 

queue_t *queue_init();
void queue_push(queue_t *q, void *element);
void *queue_front(queue_t *q); 
void *queue_rear(queue_t *q); 
void *queue_pop(queue_t *q); 
void *queue_find(queue_t *q, int (*compare)(void *target, void *value), void *value);
int queue_size(queue_t *q);


#endif


