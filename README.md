
Simple repo for querying a prometheus db. Inspired by the Linkerd dashboard

The prometheus target MUST use the provided scraper config.

Example json blob
```json
{
  "Nodes": [
    {
      "app": "vote-bot",
      "version": "",
      "namespace": "emojivoto",
      "stats": {
        "p50ms": "0",
        "p90ms": "0",
        "p99ms": "0",
        "rps": "0",
        "successRate": "0"
      }
    },
    {
      "app": "web-svc",
      "version": "",
      "namespace": "emojivoto",
      "stats": {
        "p50ms": "5.5",
        "p90ms": "10",
        "p99ms": "18",
        "rps": "2",
        "successRate": "1"
      }
    }
  ],
  "Edges": [
    {
      "fromNamespace": "emojivoto",
      "fromApp": "vote-bot",
      "fromVersion": "",
      "toNamespace": "emojivoto",
      "toApp": "web-svc",
      "toVersion": "",
      "stats": {
        "p50ms": "7.142857142857143",
        "p90ms": "14.999999999999995",
        "p99ms": "19",
        "rps": "2",
        "successRate": "1"
      }
    },
    {
      "fromNamespace": "emojivoto",
      "fromApp": "web-svc",
      "fromVersion": "",
      "toNamespace": "emojivoto",
      "toApp": "emoji-svc",
      "toVersion": "",
      "stats": {
        "p50ms": "1.4782608695652173",
        "p90ms": "4.249999999999999",
        "p99ms": "4.849999999999999",
        "rps": "3",
        "successRate": "1"
      }
    },
    {
      "fromNamespace": "emojivoto",
      "fromApp": "web-svc",
      "fromVersion": "",
      "toNamespace": "emojivoto",
      "toApp": "voting-svc",
      "toVersion": "",
      "stats": {
        "p50ms": "1.4782608695652173",
        "p90ms": "4.249999999999999",
        "p99ms": "4.849999999999999",
        "rps": "3",
        "successRate": "1"
      }
    }
  ]
}
```
