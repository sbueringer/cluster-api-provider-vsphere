# Third party

This package vendors multiple packages from core Cluster API. Either because they are internal or because we require changes.

* ./controllers/topology/cluster/patches/variables (from internal/controllers/topology/cluster/patches/variables)
  * Had to be vendored because it is internal.
  * No modifications were made apart from dropping some functions to avoiding transitive dependencies.
* ./exp/runtime/server
  * server.Server was modified to satisfy the CR webhook.Server interface so we can pass it into the CR Manager.
* ./exp/runtime/topologymutation
  * WalkTemplates was modified to provide typed and builtin variables.
