// Vkpipe by DomesticMoth
//
// To the extent possible under law, the person who associated CC0 with
// Vkpipe has waived all copyright and related or neighboring rights
// to Vkpipe.
//
// You should have received a copy of the CC0 legalcode along with this
// work.  If not, see <http://creativecommons.org/publicdomain/zero/1.0/>.

package vkpipe

import (
	"time"
	"context"
	"encoding/base64"
	"github.com/SevereCloud/vksdk/v2/api"
	"github.com/SevereCloud/vksdk/v2/longpoll-bot"
	vkevents "github.com/SevereCloud/vksdk/v2/events"
	"github.com/SevereCloud/vksdk/v2/api/params"
	log "github.com/sirupsen/logrus"
)

type Bot struct{
	Token string
	Peer int
}

type Message struct{
	Msg string
	Nom uint64
}

type Sender struct{
	vkapi api.VK
	peer int
}

type VkPipe struct{
	listener *longpoll.LongPoll
	senders []Sender
	incChan chan []byte
	outChan chan []byte
	incRawChan chan Message
	incRawRawChan chan Message
	incLim int
	outLim int
	errChan chan error
	stamps []string
	stamp int
	nom uint64
	sendersChan chan *Sender
	sends int
}

func NewVkPipe(inc Bot, out []Bot, incChan, outChan chan []byte) (vk VkPipe, err error){
	errChan := make(chan error)
	stamps := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", 
					   "a", "b", "c", "d", "e", "f", "g", "h", "i", "j",
					   "k", "l", "m", "n", "o", "p", "q", "r", "s", "t",
					   "u", "v", "w", "x", "y", "z", "+", "-", "<", ">"}
	incRawChan := make(chan Message, len(stamps))
	incRawRawChan := make(chan Message)
	
	incvk := api.NewVK(inc.Token)
	group, err := incvk.GroupsGetByID(nil)
	if err != nil { return }
	listener, err := longpoll.NewLongPoll(incvk, group[0].ID)
	if err != nil { return }
	senders := []Sender{}
	sendersChan := make(chan *Sender, len(out))
	for _, bot := range out{
		sender := Sender{*api.NewVK(bot.Token), bot.Peer}
		sendersChan <- &sender
	}
	
	vk = VkPipe{
		listener,
		senders,
		incChan,
		outChan,
		incRawChan,
		incRawRawChan,
		0,
		3,
		errChan,
		stamps,
		0,
		0,
		sendersChan,
		0,
	}

	listener.MessageNew(func(_ context.Context, obj vkevents.MessageNewObject) {
		if obj.Message.PeerID == inc.Peer {
			//log.Trace("Received", obj.Message.Text)
			incRawRawChan <- Message{obj.Message.Text, 0}
		}
	})

	vk.listener = listener
	log.Trace("Created")
	return
}

func (pipe * VkPipe) nomer(){
	for{
		msg := <- pipe.incRawRawChan
		msg.Nom = pipe.nom
		pipe.incRawChan <- msg
	}
}

func (pipe * VkPipe) listen(){
	err := (*pipe.listener).Run()
	if err != nil {
		pipe.errChan <- err
	}
}

func (pipe * VkPipe) send(msg string) error {
	if pipe.sends >= pipe.outLim {
		pipe.sends = 0
		time.Sleep(1 * time.Second)
	}
	pipe.sends += 1
	sender := <- pipe.sendersChan
	pipe.sendersChan <- sender

	b := params.NewMessagesSendBuilder()
	b.Message(msg)
	b.RandomID(0)
	b.PeerID(sender.peer)
	_, err := sender.vkapi.MessagesSend(b.Params)
	return err
}

func (pipe * VkPipe) nextStamp(stamp int) int {
	if stamp >= len(pipe.stamps)-1{
		stamp = 0
	}else{
		stamp += 1
	}
	return stamp
}

func (pipe * VkPipe) Run(ctx context.Context) error {
	sendstamp := 0
	recvstamp := 0
	go pipe.nomer()
	defer (*pipe.listener).Shutdown()
	go pipe.listen()
	err := pipe.send("?")
	if err != nil { return err }
	time.Sleep(1 * time.Second)
	log.Trace("Start")
	for {
		select {
			case <-ctx.Done():
				return nil
			case err := <- pipe.errChan:
				return err
			case rawMsgWrap := <- pipe.incRawChan:
				log.Trace("Received", rawMsgWrap)
				rawMsg := rawMsgWrap.Msg
				if rawMsg == "" { continue }
				if rawMsg == "?" {
					recvstamp = 0
					pipe.nom += 1
					continue
				}
				if rawMsgWrap.Nom != pipe.nom {
					log.Trace("Dropped", rawMsgWrap, pipe.nom) 
					continue
				}
				rawStmp := string(rawMsg[0])
				stmp := pipe.textStampToInt(rawStmp)
				if stmp != recvstamp{
					pipe.incRawChan <- rawMsgWrap
					continue
				}
				recvstamp = pipe.nextStamp(recvstamp)
				msg := rawMsg[1:]
				data, err := base64.StdEncoding.DecodeString(msg)
				if err != nil { return err }
				log.Trace("Give", data)
				pipe.incChan <- data
				log.Trace("Gived", data)
			case outerMsg := <- pipe.outChan:
				str := base64.StdEncoding.EncodeToString(outerMsg)
				str = pipe.intStampToText(sendstamp) + str
				sendstamp = pipe.nextStamp(sendstamp)
				err := pipe.send(str)
				if err != nil { return err }
		}
	}
}

func (pipe * VkPipe) textStampToInt(stamp string) int{
	for ret, stmp := range pipe.stamps {
		if stmp == stamp {
			return ret
		}
	}
	return 0
}

func (pipe * VkPipe) intStampToText(stamp int) string{
	if stamp >= len(pipe.stamps) {return "!"}
	return pipe.stamps[stamp]
}
