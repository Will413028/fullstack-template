# Atlas: each module's desired-state schema (internal/modules/<m>/schema.sql) -> one migrations/ directory.
# auth loads first: items.owner_id -> users.id is the only cross-module foreign key allowed.
locals {
  auth_schema  = "internal/modules/auth/schema.sql"
  schema_files = concat(
    [for f in fileset(local.auth_schema) : f],
    [for f in fileset("internal/modules/*/schema.sql") : f if !endswith(f, local.auth_schema)],
  )
}

# Generating migrations (make migration) and drift checks; needs Docker and the standard build (make tools).
env "dev" {
  src = [for f in local.schema_files : "file://${f}"]
  dev = "docker://postgres/18/dev"
  migration {
    dir = "file://migrations"
  }
  diff {
    # Indexes on existing tables are created and dropped CONCURRENTLY, with `-- atlas:txmode none` added to the
    # file header (only the standard build adds it).
    concurrent_index {
      create = true
      drop   = true
    }
  }
}
