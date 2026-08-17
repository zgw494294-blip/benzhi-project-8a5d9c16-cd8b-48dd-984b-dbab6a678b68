# BagHold

BagHold is a small in-memory HTTP JSON service for composite fabrication vacuum-bag hold tests. A technician creates a hold test, records ordered vacuum readings, assesses the hold once the configured duration is reached, and retrieves the immutable result.

## Run

Start the service on port 8080:

```text
go run ./cmd/baghold
```

Run the bounded end-to-end smoke workflow:

```text
go run ./cmd/baghold smoke
```

Records live only in the running process.

## API

Create a test with `POST /tests`:

```json
{
  "bag_id": "HULL-07",
  "min_hold_seconds": 3600,
  "max_vacuum_loss_kpa": 2.0,
  "operator_note": "starboard repair"
}
```

Append readings with `POST /tests/{id}/samples`:

```json
{
  "elapsed_seconds": 1800,
  "vacuum_kpa": 29.4
}
```

Assess with `POST /tests/{id}/assess` after at least two readings and the configured duration. Retrieve the complete record with `GET /tests/{id}`. Assessment status is `passed` when the first-to-last vacuum loss is within the configured limit, otherwise it is `failed`.

The service rejects unknown JSON fields, invalid readings, out-of-order samples, missing records, and mutations after assessment with stable JSON error objects.
