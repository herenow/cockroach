* Construct a base handler for all HTTP servers to glog.Fatal the
  process with a deferred recover func to prevent HTTP from swallowing
  panics which might otherwise be holding locks, etc.

* Transactions

  - Keep a cache of pushed transactions on a store to avoid repushing
    further intents after a txn has already been aborted or its
    timestamp moved forward.

* Redirect clients if HTTP server is busy compared to others in the
  cluster. Report node's load via gossip as part of a max
  group. Measure node's load using a decaying stat. Verify redirect
  behavior with http client.

* Write a test for transaction starvation.

* Change key / end key in request to a more flexible list of key
  intervals to properly account for batch requests. This data is
  used for command queue, timestamp cache and response cache.
  Currently, key/end key is the union interval of all sub requests
  in a batch request, meaning there's less allowable concurrency
  in cases where keys affected by the batch are widely separated.

* Allow atomic update batches to replace transactions where possible.
  This means batching up BeginTransaction request with first batch via
  TxnSender. In the event BeginTransaction is the first request in the
  batch and EndTransaction is the last, we want to let the DistSender
  determine whether all are on the same range and if so, execute
  entire batch as a single batch command to the range. In all other
  cases, we split batch request at DistSender into individual requests.

  In this context, revisit the handling of (potentially) range-spanning
  operations such as Scan, DeleteRange, InternalResolveIntent.
  Currently, these are wrapped in a txn in txn_coord_sender and re-sent
  using a one-off client.KV unless already in transactional context.
  This could be more transparently implemented by

  - auto-wrapping all such operations in a Txn which is (usually) optimized
    away unless the cmd actually spans ranges
  - carrying out InternalResolveIntent across ranges without a txn, which might
    be okay since resolving intents is best-effort and does not require
    transactional semantics (in fact, might be better off without)

* Propagate errors from storage/id_alloc.go

* Accept a list of acknowledged client command ids in RequestHeader,
  allowing the server to garbage collect response cache entries.

* StoreFinder using Gossip protocol to filter

* Rebalance range replica. Only fully-replicated ranges may be
  rebalanced.

  - Keep a rebalance queue in memory. Range replicas are added to the
    queue from a store during initial range scan and also during
    operation as a response to certain conditions. Listed here:

    - A range is split. Each replica in the split range is marked as
      needing rebalancing.

    - Replica not matching zone config. When zone config changes happen,
      all ranges are scanned by each store and any mismatched replicas
      are added to the queue.

  - Rebalance away from stores finding themselves in top N space
    utilized, taking care to account for fewer than N stores in
    cluster. Only stores finding themselves in the top N space
    utilized set may have rebalances in effect.

  - Rebalance target is selected from available stores in bottom N
    space utilized. Adjacent stores are exempted.

  - Add rebalance target to replica set and rewrite range addressing
    indexes.

  - Rebalance targets are added to replica set always exactly one at a
    time. Targets are marked as REBALANCING. Obsolete sources are
    marked as PENDING_DELETION. Any time a range becomes fully
    replicated, the range leader replica will move REBALANCING
    replicas into state OK and will remove PENDING_DELETION replicas
    from the RangeDescriptor. The store which owns a removed replica
    is responsible for clearing the relevant portion of the key space
    as well as any other housekeeping details.

* Cleanup proto files to adhere to proto capitalization instead of go's.

* Rewrite storage/engine/batch.go functionality to C++, using Viewfinder
  client C++ code as a starting point. Snapshots and batches should just
  be new engine types.
