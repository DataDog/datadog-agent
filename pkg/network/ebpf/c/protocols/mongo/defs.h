#ifndef __MONGO_DEFS_H
#define __MONGO_DEFS_H

// Reference:
// https://docs.mongodb.com/manual/reference/mongodb-wire-protocol/#std-label-wp-request-opcodes.
// Note: Response side inference for Mongo is not robust, and is not attempted to avoid
// confusion with other protocols, especially MySQL.
#define MONGO_OP_REPLY 1
#define MONGO_OP_UPDATE 2001
#define MONGO_OP_INSERT 2002
#define MONGO_OP_RESERVED 2003
#define MONGO_OP_QUERY 2004
#define MONGO_OP_GET_MORE 2005
#define MONGO_OP_DELETE 2006
#define MONGO_OP_KILL_CURSORS 2007
#define MONGO_OP_COMPRESSED 2012
#define MONGO_OP_MSG 2013

#define MONGO_HEADER_LENGTH 16

#endif
