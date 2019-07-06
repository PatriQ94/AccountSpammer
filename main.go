package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/iotaledger/iota.go/account/deposit"
	"github.com/iotaledger/iota.go/account/event/listener"
	"github.com/iotaledger/iota.go/bundle"

	"github.com/iotaledger/iota.go/account"
	"github.com/iotaledger/iota.go/account/Store/badger"
	"github.com/iotaledger/iota.go/account/builder"
	"github.com/iotaledger/iota.go/account/event"
	"github.com/iotaledger/iota.go/account/timesrc"
	. "github.com/iotaledger/iota.go/api"
	"github.com/iotaledger/iota.go/pow"
)

//Global variables
var (
	Seed1              = "NIXIDSXHTTAHDCGNHRQOIHEKDKBNHYTY9FYQSGYNOACACOKWNBHPFZZZZMKHDEUPOTWMCVAHMXA9XNBPM"
	Seed2              = "CL9YNQZKNCFDCFZCEPBNH9ATYJIVEDOIVCEPFP9XA9GRHYKZZXVHP9KNIQAVNDQF9TYXEPPLPOTLNSQRX"
	Store              badger.BadgerStore
	sendingWaitGroup   = &sync.WaitGroup{}
	Account1           account.Account
	Account2           account.Account
	IotaAPI            *API
	CurrentlySending   account.Account
	CurrentlyReceiving account.Account
	SendingInProgress  bool
	Timesource         = timesrc.NewNTPTimeSource("time.google.com")
)

func main() {
	//Initialize storage
	Store, err := badger.NewBadgerStore("D:\\Repositories\\golearning\\AccountDB")
	must(err)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, os.Kill)
	go func() {
		sig := <-ch // waits for death signal
		Store.Close()
		fmt.Println("Captured", sig, "signal stopping and exiting..")
		os.Exit(0)
	}()

	//Initialize API
	_, powFunc := pow.GetFastestProofOfWorkImpl()
	IotaAPI, err := ComposeAPI(HTTPClientSettings{URI: "https://nodes.devnet.thetangle.org:443/", LocalProofOfWorkFunc: powFunc})
	must(err)

	emAcc1 := event.NewEventMachine()
	emAcc2 := event.NewEventMachine()

	//Make accounts
	Account1, err := builder.NewBuilder().WithAPI(IotaAPI).WithStore(Store).WithSeed(Seed1).WithMWM(9).WithTimeSource(Timesource).WithEvents(emAcc1).WithDefaultPlugins().Build()
	must(err)
	Account2, err := builder.NewBuilder().WithAPI(IotaAPI).WithStore(Store).WithSeed(Seed2).WithMWM(9).WithTimeSource(Timesource).WithEvents(emAcc2).WithDefaultPlugins().Build()
	must(err)

	//Start accounts
	must(Account1.Start())
	defer Account1.Shutdown()
	must(Account2.Start())
	defer Account2.Shutdown()

	//Initialize listeners
	lis1 := listener.NewChannelEventListener(emAcc1).RegReceivedDeposits().RegSentTransfers()
	lis2 := listener.NewChannelEventListener(emAcc2).RegReceivedDeposits().RegSentTransfers()

	//Listener for Account1
	go func() {
		for {
			select {
			case bndl := <-lis1.ReceivedDeposit:
				fmt.Println("Account 1 > Received deposit on:", bndl[0].Address)
				sendingWaitGroup.Done()
			case bndl := <-lis1.SentTransfer:
				fmt.Println("Account 1 > Sending to:", bndl[0].Address)
			}
		}
	}()

	//Listener for Account2
	go func() {
		for {
			select {
			case bndl := <-lis2.ReceivedDeposit:
				fmt.Println("Account 2 > Received deposit on:", bndl[0].Address)
				sendingWaitGroup.Done()
			case bndl := <-lis2.SentTransfer:
				fmt.Println("Account 2 > Sending to:", bndl[0].Address)
			}
		}
	}()

	//Print their initial balances
	balance1, err := Account1.TotalBalance()
	must(err)
	balance2, err := Account2.TotalBalance()
	must(err)

	//Set expiration times for CDAs
	now, err := Timesource.Time()
	must(err)
	now = now.Add(time.Duration(72) * time.Hour)

	//If both accounts are empty, first the user has to deposit funds on an account in order to continue
	if balance1 == 0 && balance2 == 0 {
		// allocate a new CDA
		conditions := &deposit.Conditions{TimeoutAt: &now, MultiUse: false}
		cda, err := Account1.AllocateDepositAddress(conditions)
		must(err)
		fmt.Println("Please first deposit funds to this address in order to use the spammer:" + cda.Address)
		for {
			balance1, err := Account1.TotalBalance()
			must(err)
			if balance1 != 0 {
				fmt.Println("Balance of", balance1, "has been deposited on address", cda.Address)
				break
			}
			time.Sleep(1 * time.Second)
		}
	} else {
		fmt.Println("Current balance of Account1 is", balance1, ", balance of Account2 is:", balance2)
	}

	balance1, err = Account1.TotalBalance()
	must(err)
	balance2, err = Account2.TotalBalance()
	must(err)

	if balance1 > 0 {
		fmt.Println("Starting with Account 1!")
		Spam(Account1, Account2, now)
	} else {
		fmt.Println("Starting with Account 2!")
		Spam(Account2, Account1, now)
	}
}

//Method of spamming with two accounts:
//If sender account has balance of 1000, generate 1 receiver deposit address (CDA), send 1000 * 1i to that address, then switch accounts
func Spam(CurrentSender account.Account, CurrentReceiver account.Account, now time.Time) {
	for {
		senderBalance, err := CurrentSender.AvailableBalance()
		must(err)

		if senderBalance == 0 {
			tmp := CurrentSender
			CurrentSender = CurrentReceiver
			CurrentReceiver = tmp
			fmt.Println("SWITCHING SENDER AND RECEIVER!")
		} else {
			fmt.Println("Current sender ID:", CurrentSender.ID()[:5], ", current receiver ID:", CurrentReceiver.ID()[:5], ", number of transactions to send: ", +senderBalance)
			var amount = senderBalance

			//Get receiver CDA address
			conditions := &deposit.Conditions{TimeoutAt: &now, ExpectedAmount: &amount}
			cda, err := CurrentReceiver.AllocateDepositAddress(conditions)
			must(err)

			//Transfer object
			transf := bundle.Transfer{
				Address: cda.Address,
				Value:   1,
			}

			//Start spamming an address
			for i := 1; i <= int(amount); i++ {
				fmt.Println("Before sent", i, "/", amount)
				sendingWaitGroup.Add(1)
				_, err := CurrentSender.Send(transf)
				if err != nil {
					fmt.Printf("Error sending: %s\n", err.Error())
					Store.Close()
					os.Exit(0)
				}

				//Wait for the account to receive
				sendingWaitGroup.Wait()
				fmt.Println("After sent", i, "/", amount)
			}
		}
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
