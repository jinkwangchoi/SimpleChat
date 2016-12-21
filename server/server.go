package main

import (
	"errors"
	"fmt"
	. "github.com/jinkwangchoi/chat/chat"
	"google.golang.org/grpc"
	"log"
	"net"
)

type chatServer struct {
	opChan chan syncFunc
}

type user struct {
	id     int64
	name   string
	stream Chat_ChatServer
}
type syncFunc func(nextClientID *int64, clients map[int64]user, userNameDic map[string]int64)

func newChatServer() *chatServer {
	cs := &chatServer{
		opChan: make(chan syncFunc, 100),
	}

	go cs.run()
	return cs
}

func (cs *chatServer) run() {
	var nextClientID int64
	clients := make(map[int64]user)
	userNameDictionary := make(map[string]int64)

	for {
		op := <-cs.opChan
		op(&nextClientID, clients, userNameDictionary)
	}
}

type loginResult struct {
	clientID int64
	err      error
}

func (cs *chatServer) Chat(stream Chat_ChatServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}
	if req.GetReqType() != UserRequest_USER_LOGIN {
		return errors.New("Login First")
	}

	userName := req.GetName()

	resultChan := make(chan loginResult, 1)
	cs.opChan <- func(nextClientID *int64, clients map[int64]user, userNameDic map[string]int64) {
		if len(userName) == 0 {
			resultChan <- loginResult{
				err: errors.New("Empty Name"),
			}
			return
		}

		if _, ok := userNameDic[userName]; ok {
			resultChan <- loginResult{
				err: errors.New("Name Duplicated"),
			}
			return
		}

		clientID := *nextClientID
		userNameDic[userName] = clientID

		clients[clientID] = user{
			id:     clientID,
			name:   userName,
			stream: stream,
		}
		*nextClientID++
		resultChan <- loginResult{
			clientID: clientID,
		}
	}

	result := <-resultChan
	if result.err != nil {
		return result.err
	}

	err = cs.sendOtherUsersJoinTo(result.clientID)
	if err != nil {
		fmt.Errorf(err.Error())
	}

	err = cs.broadcastUserJoin(result.clientID, userName)
	if err != nil {
		fmt.Errorf(err.Error())
	}

	for {
		req, err := stream.Recv()
		if err != nil {
			fmt.Errorf(err.Error())
			return cs.processUserLeave(result.clientID)
		}
		switch req.GetReqType() {
		case UserRequest_USER_LOGIN:
			return errors.New("Already Login")

		case UserRequest_USER_CHAT:
			err := cs.broadcastUserChat(result.clientID, req.GetChat())

			if err != nil {
				fmt.Errorf(err.Error())
			}
		}
	}

	return nil
}

func (cs *chatServer) sendOtherUsersJoinTo(targetUserID int64) error {
	errorChan := make(chan error, 1)
	cs.opChan <- func(nextClientID *int64, clients map[int64]user, userNameDic map[string]int64) {
		targetClient, ok := clients[targetUserID]
		if !ok {
			errorChan <- errors.New("Target User Not Found")
			return
		}
		for userID, client := range clients {
			if userID == targetUserID {
				continue
			}
			err := targetClient.stream.Send(&RoomMsg{
				MsgType:       RoomMsg_USER_JOIN,
				UserId:        userID,
				RoomMsgValues: &RoomMsg_Name{Name: client.name},
			})
			if err != nil {
				errorChan <- err
				return
			}
		}
		errorChan <- nil
	}
	err := <-errorChan
	return err
}

func (cs *chatServer) broadcastUserJoin(userID int64, userName string) error {
	return cs.broadcastRoomMsg(RoomMsg{
		MsgType:       RoomMsg_USER_JOIN,
		UserId:        userID,
		RoomMsgValues: &RoomMsg_Name{Name: userName},
	})
}

func (cs *chatServer) broadcastUserChat(senderID int64, msg string) error {
	return cs.broadcastRoomMsg(RoomMsg{
		MsgType:       RoomMsg_USER_CHAT,
		UserId:        senderID,
		RoomMsgValues: &RoomMsg_Msg{Msg: msg},
	})
}

func (cs *chatServer) broadcastRoomMsg(roomMsg RoomMsg) error {
	errorChan := make(chan error, 1)
	cs.opChan <- func(nextClientID *int64, clients map[int64]user, userNameDic map[string]int64) {
		for _, client := range clients {
			err := client.stream.Send(&roomMsg)
			if err != nil {
				errorChan <- err
				return
			}
		}
		errorChan <- nil
	}
	err := <-errorChan
	return err
}

func (cs *chatServer) processUserLeave(userID int64) error {
	if err := cs.removeUser(userID); err != nil {
		return err
	}
	return cs.broadcastRoomMsg(RoomMsg{
		MsgType: RoomMsg_USER_LEAVE,
		UserId:  userID,
	})
}

func (cs *chatServer) removeUser(userID int64) error {
	errorChan := make(chan error, 1)
	cs.opChan <- func(nextClientID *int64, clients map[int64]user, userNameDic map[string]int64) {
		client, ok := clients[userID]
		if !ok {
			errorChan <- errors.New("User Not Found")
			return
		}

		delete(userNameDic, client.name)
		delete(clients, userID)
		errorChan <- nil
	}
	err := <-errorChan
	return err
}

func main() {
	lis, err := net.Listen("tcp", ":12345")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	RegisterChatServer(grpcServer, newChatServer())
	grpcServer.Serve(lis)
}
