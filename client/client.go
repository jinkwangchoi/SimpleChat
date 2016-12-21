package main

import (
	"bufio"
	"flag"
	"fmt"
	. "github.com/jinkwangchoi/chat/chat"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
	"os"
)

func main() {
	userName := flag.String("user_name", "Unamed", "user name")
	flag.Parse()

	conn, err := grpc.Dial("127.0.0.1:12345", grpc.WithInsecure())
	if err != nil {
		grpclog.Fatalf("fail to dial: %v", err)
		return
	}
	defer conn.Close()
	client := NewChatClient(conn)
	stream, err := client.Chat(context.Background())
	if err != nil {
		grpclog.Fatalf("fail to chat: %v", err)
		return
	}

	defer func() {
		fmt.Println("CloseSend")
		stream.CloseSend()
	}()

	userMap := make(map[int64]string)
	go func() {
		for {
			roomMsg, err := stream.Recv()
			if err != nil {
				grpclog.Fatalf("fail to receive: %v", err)
				return
			}
			switch roomMsg.MsgType {
			case RoomMsg_USER_JOIN:
				userMap[roomMsg.UserId] = roomMsg.GetName()
				fmt.Printf("%s Joined\n", roomMsg.GetName())
			case RoomMsg_USER_CHAT:
				userName, ok := userMap[roomMsg.UserId]
				if !ok {
					userName = "UnknownUser"
				}
				fmt.Printf("%s: %s\n", userName, roomMsg.GetMsg())
			case RoomMsg_USER_LEAVE:
				userName, ok := userMap[roomMsg.UserId]
				if ok {
					fmt.Printf("%s Left\n", userName)
					delete(userMap, roomMsg.UserId)
				}
			}
		}
	}()

	err = stream.Send(&UserRequest{
		ReqType:           UserRequest_USER_LOGIN,
		UserRequestValues: &UserRequest_Name{Name: *userName},
	})
	if err != nil {
		grpclog.Fatalf("fail to login: %v", err)
		return
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		readLine, _, err := reader.ReadLine()
		if err != nil {
			grpclog.Fatalf("ReadLine Error : %s", err.Error())
			return
		}
		if len(readLine) == 0 {
			fmt.Println("End Chat")
			return
		}
		err = stream.Send(&UserRequest{
			ReqType:           UserRequest_USER_CHAT,
			UserRequestValues: &UserRequest_Chat{Chat: string(readLine)},
		})
		if err != nil {
			grpclog.Fatalf("fail to send chat: %v", err)
			return
		}
	}
}
