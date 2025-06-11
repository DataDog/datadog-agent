
# Approvers to kernel filters

```
            ┌────────────────────────┐                                              
            │                        │                                              
            │   Rule engine          │                                              
        ┌───►   Approvers detection  ┼───┐                                          
        │   │                        │   │                                          
        │   └────────────────────────┘   │                                          
        │                                │                                          
        │                                │                                          
        │                                │                                          
        │                                │                                          
┌───────┼───────────┐          ┌─────────▼─────────┐                                
│                   │          │                   │                                
│    Capabilities   │          │    FilterReport   ┼──────────────────────┐         
│                   │          │                   │            ┌─────────▼────────┐
└───────────────────┘          └───────────────────┘            │                  │
                                                                │                  │
                                                                │    Probe eBPF    │
                                                                │                  │
┌───────────────────┐          ┌───────────────────┐            │                  │
│                   │          │                   │            └─────────┬────────┘
│      KFilters     ◄──────────┼   KFilterGetters  ◄──────────────────────┘         
│                   │          │                   │                                
└─────────┬─────────┘          └───────────────────┘                                
          │                                                                         
          │                                                                         
┌─────────▼─────────┐                                                               
│                   │                                                               
│       eBPF        │                                                               
│                   │                                                               
└───────────────────┘                                                               

```

* [Edit Schema](https://asciiflow.com/#/share/eJztlb1qwzAQx1%2FFaM5U2kC9tYV06BI6a3HaowhU2chKSQiBEDp2yGDSPESfoPhp8iSVY0rtNNaHLXWpD4Pts%2FTz%2F6S70wKx6BlQyKaUDhCN5sBRiBYYvQBPScwwCs8GGM3k%2FfL8Qj7NC89wKJ8EzIR8wSio2D5722crR9cmsDKMWV3Kummk4pMFmk8pBMCeCAM36NribT%2Bl5ypJeFxsRfAIAh6E3JFiWN5ukWq%2FWqvFagcY4TN3ybDrLEdpPbojurnyc4PdrWSxpoVs1ThtORykngqt5jUYosPfREk0IZQIAmkDfkSoAH4PScxFcFzYDtqmw1g77MrmIKNLK9hVdTgCNcQtnUfNvo35R495PJGnz%2FV45Bz9y6msbruM1OWROUgj11XWq7PtQ5FqVRl3ZaWnJfT9VRdn%2FjPnFkQ502CaXQn8RSdsvXwrX1XfINUqNGPzCfZ8TpoIaJkZNujvDucBXTVHaK9ZqhfQkdBje2yP%2FfdYeXTowWiJll8vsDrX)