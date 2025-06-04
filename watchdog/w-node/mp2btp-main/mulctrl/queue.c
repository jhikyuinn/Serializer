#include "queue.h"

queue_t *queue_init()
{
    queue_t *q = (queue_t *) malloc(sizeof(queue_t));
    q->size = 0; 
    q->head = q->tail = NULL;
    pthread_mutex_init(&q->mutex, NULL);

    return q;
}

void queue_push(queue_t *q, void *element)
{
    queue_node_t *new_node = (queue_node_t*) malloc(sizeof(queue_node_t));
    if (new_node == NULL)
        return;

    pthread_mutex_lock(&q->mutex);
    new_node->data = element;
    new_node->next = NULL;

    if (q->head) 
        q->tail->next = new_node; 
    else 
        q->head = new_node; 

    q->tail = new_node; 
    q->size++;
    pthread_mutex_unlock(&q->mutex);
}

void *queue_front(queue_t *q) 
{
    void *data = NULL;
    pthread_mutex_lock(&q->mutex);
    if (q->size > 0)
        data = q->head->data;
    pthread_mutex_unlock(&q->mutex);
    return data;
    //return q->size > 0 ? q->head->data : NULL;
}

void *queue_rear(queue_t *q)
{
    void *data = NULL;
    pthread_mutex_lock(&q->mutex);
    if (q->size > 0)
        data = q->tail->data;
    pthread_mutex_unlock(&q->mutex);
    return data;
    //return q->size > 0 ? q->tail->data : NULL;
}

void *queue_pop(queue_t *q)
{
    void *element = NULL; 

    if (q->size > 0) 
    {   
        pthread_mutex_lock(&q->mutex);
        element = q->head->data;
        q->head = q->head->next;
        
        if (q->size == 1)
            q->tail = NULL;

        // Not release memory 
        q->size--;
        pthread_mutex_unlock(&q->mutex);
    }  
    
    return element; 
}

void *queue_find(queue_t *q, int (*compare)(void *target, void *search), void *search)
{
    queue_node_t *cur = q->head; 

    while (cur != NULL) 
    {
        if (compare(cur->data, search)) 
            return cur->data; 
        else 
            cur = cur->next;
    }
    return NULL;
}

int queue_size(queue_t *q)
{
    return q->size;
}


