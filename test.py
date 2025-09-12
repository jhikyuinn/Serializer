from pymongo import MongoClient
from pymongo.errors import PyMongoError

def get_latest_abort_transaction():
    try:
        client = MongoClient("mongodb://117.16.244.33:27017", serverSelectionTimeoutMS=10000)

        database = client["User"]
        collection = database["abortTransaction"]

        result = collection.find_one(sort=[("_id", -1)])

        if result is None:
            print("No documents found")
            return None

        abort_count = result.get("NumofAbortTransaction", None)
        return abort_count

    except PyMongoError as e:
        return None

    finally:
        client.close()

if __name__ == "__main__":
    abort_count = get_latest_abort_transaction()
    if abort_count is not None:
        print("Latest abort transaction count: {abort_count}")