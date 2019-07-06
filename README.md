# AccountSpammer
IOTA Spammer built using the Account Module (https://github.com/iotaledger/iota.go) 
It generates two Accounts that ping pong value transfer between each other. 
The current implementation is not optimal because it relies on confirmations of transactions meaning
that an issued transaction has to be confirmed in order for the next transaction to be issued.
