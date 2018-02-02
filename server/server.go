package main

import (
	"flag"
	"net"
	"fmt"
	"log"
	"encoding/json"
	"bufio"
	"unicode"
	"time"
	"math/rand"
	"os"
	"strings"
	"strconv"
)

const (
	AlohaCommand      = 1 << iota
	LeaveCommand
	NewMessageCommand
	//FileTransferCommand
	//FileMetaInfo
)

const (
	MessageDelimiter          byte   = '\n'
	ServerMaxQueue            uint32 = 1 << 7
	ServerFilesRootFolderPath string = "files/"
	MaxBufferSize             uint64 = 1024 * 1024
)

func init() {
	os.Mkdir(ServerFilesRootFolderPath, os.ModePerm) // chmod +x rwxrwxrwx
}

type Server struct {
	clients  map[net.Addr]*Client // map<ip, client>
	joins    chan net.Conn
	leaves   chan *Client
	incoming chan *Message
	outgoing chan *Message
}

type Client struct {
	server     *Server
	username   string
	connection net.Conn
	reader     *bufio.Reader
	writer     *bufio.Writer
}

type Message struct {
	MessageType int    `json:"type"`
	Content     string `json:"content"`
	Sender      string `json:"sender"` // embedded Client type?
	senderIP    net.Addr
	Timestamp   string `json:"timestamp"`
}

func newServer() *Server {
	server := &Server{
		clients:  make(map[net.Addr]*Client),
		joins:    make(chan net.Conn, ServerMaxQueue),
		leaves:   make(chan *Client, ServerMaxQueue),
		incoming: make(chan *Message, ServerMaxQueue),
		outgoing: make(chan *Message, ServerMaxQueue),
	}
	return server
}

func (s *Server) Listen() {
	for {
		select {
		case client := <-s.leaves:
			// remove client
			s.deleteClient(client)
		case newConnection := <-s.joins:
			// add new client
			s.addClient(NewClient(newConnection, s))
		case message := <-s.outgoing:
			// broadcasting
			b, err := json.Marshal(*message)
			if err != nil {
				log.Fatal(err)
			}
			jsonMessage := string(b)
			for ip, client := range s.clients {
				if ip != message.senderIP {
					go s.sendMessageToClient(client, &jsonMessage)
				}
			}
		case message := <-s.incoming:
			// handle messages
			log.Println("Got new message: ", message)
			switch message.MessageType {
			case AlohaCommand:
				log.Println("Updating username for", message.senderIP)
				s.clients[message.senderIP].username = message.Sender
			case NewMessageCommand:
				log.Println("Broadcasting message: ", message, "to", len(s.clients)-1, "users")
				s.outgoing <- message
			case LeaveCommand:
				if cl, ok := s.getClient(message.senderIP); ok {
					s.leaves <- cl
				}
				//case FileTransferCommand:
				//	continue
			}
		}
	}
}

func (s *Server) sendMessageToClient(client *Client, message *string) {
	// troubles with duplication memory on messages object and converting to bytes "string -> bytes" in each goroutine
	log.Println("Sending message to ", client.username, "on", client.connection.RemoteAddr())
	err := client.Write(message)
	if err != nil {
		log.Println(err)
	}
}

func (s *Server) addClient(client *Client) {
	log.Println("Adding new client to map", client)
	s.clients[client.connection.RemoteAddr()] = client
}

func (s *Server) deleteClientByIP(addr net.Addr) {
	log.Println("Removing client from map by ip", addr)
	delete(s.clients, addr)
}

func (s *Server) deleteClient(client *Client) {
	log.Println("Removing client from map", client)
	s.deleteClientByIP(client.connection.RemoteAddr())
}

func (s *Server) getClient(addr net.Addr) (*Client, bool) {
	client, ok := s.clients[addr]
	return client, ok
}

func (s *Server) downloadFile(conn net.Conn) error {
	defer conn.Close()
	//if err := conn.SetDeadline(time.Now().Add(time.Minute)); err != nil {
	//	log.Println(err)
	//	return err
	//}
	reader := bufio.NewReader(conn)
	minfo, err := reader.ReadString(MessageDelimiter)
	ss := strings.Split(minfo, " ")
	name := ss[0]
	size, _ := strconv.ParseUint(ss[1], 10, 64)
	var curByte uint64
	fBuffer := make([]byte, MaxBufferSize)
	file, err := os.Create(ServerFilesRootFolderPath + strings.TrimSpace(name))
	writer := bufio.NewWriter(file)
	defer file.Close()
	if err != nil {
		log.Println(err)
		return err
	}

	for {
		cnt, rerr := reader.Read(fBuffer)
		if rerr != nil {
			return rerr
		}
		_, werr := writer.Write(fBuffer[:cnt])
		if err := writer.Flush(); err != nil {
			return err
		}
		curByte += uint64(cnt)

		if werr != nil {
			return werr
		}
		if size >= curByte {
			break
		}
	}

	return nil
}

func NewClient(conn net.Conn, s *Server) *Client {
	client := &Client{
		server:     s,
		connection: conn,
		reader:     bufio.NewReader(conn),
		writer:     bufio.NewWriter(conn),
		username:   GenerateStupidName(),
	}

	go client.Listen()

	return client
}

func (c *Client) Read() (*Message, error) {
	ln, err := c.reader.ReadString(MessageDelimiter)
	if err != nil {
		log.Println("Disconnect user", c.username, "-> ip", c.connection.RemoteAddr())
		c.server.leaves <- c
		return nil, err
	}
	var msg Message
	err = json.Unmarshal([]byte(ln), &msg)
	if err != nil {
		return nil, err
	}
	msg.senderIP = c.connection.RemoteAddr()
	return &msg, nil
}

func (c *Client) Write(data *string) error {
	if _, err := c.writer.WriteString(*data); err != nil {
		return err
	}
	if err := c.writer.WriteByte(MessageDelimiter); err != nil {
		return err
	}
	if err := c.writer.Flush(); err != nil {
		return err
	}
	return nil
}

func (c *Client) Listen() {
	defer c.connection.Close()
	for {
		msg, err := c.Read()
		if err != nil {
			log.Println(err)
			return
		}
		c.server.incoming <- msg
	}
}

var adjectives = []string{"Black", "White", "Gray", "Brown", "Red", "Pink", "Crimson", "Carnelian", "Orange", "Yellow", "Ivory", "Cream", "Green", "Viridian", "Aquamarine", "Cyan", "Blue", "Cerulean", "Azure", "Indigo", "Navy", "Violet", "Purple", "Lavender", "Magenta", "Rainbow", "Iridescent", "Spectrum", "Prism", "Bold", "Vivid", "Pale", "Clear", "Glass", "Translucent", "Misty", "Dark", "Light", "Gold", "Silver", "Copper", "Bronze", "Steel", "Iron", "Brass", "Mercury", "Zinc", "Chrome", "Platinum", "Titanium", "Nickel", "Lead", "Pewter", "Rust", "Metal", "Stone", "Quartz", "Granite", "Marble", "Alabaster", "Agate", "Jasper", "Pebble", "Pyrite", "Crystal", "Geode", "Obsidian", "Mica", "Flint", "Sand", "Gravel", "Boulder", "Basalt", "Ruby", "Beryl", "Scarlet", "Citrine", "Sulpher", "Topaz", "Amber", "Emerald", "Malachite", "Jade", "Abalone", "Lapis", "Sapphire", "Diamond", "Peridot", "Gem", "Jewel", "Bevel", "Coral", "Jet", "Ebony", "Wood", "Tree", "Cherry", "Maple", "Cedar", "Branch", "Bramble", "Rowan", "Ash", "Fir", "Pine", "Cactus", "Alder", "Grove", "Forest", "Jungle", "Palm", "Bush", "Mulberry", "Juniper", "Vine", "Ivy", "Rose", "Lily", "Tulip", "Daffodil", "Honeysuckle", "Fuschia", "Hazel", "Walnut", "Almond", "Lime", "Lemon", "Apple", "Blossom", "Bloom", "Crocus", "Rose", "Buttercup", "Dandelion", "Iris", "Carnation", "Fern", "Root", "Branch", "Leaf", "Seed", "Flower", "Petal", "Pollen", "Orchid", "Mangrove", "Cypress", "Sequoia", "Sage", "Heather", "Snapdragon", "Daisy", "Mountain", "Hill", "Alpine", "Chestnut", "Valley", "Glacier", "Forest", "Grove", "Glen", "Tree", "Thorn", "Stump", "Desert", "Canyon", "Dune", "Oasis", "Mirage", "Well", "Spring", "Meadow", "Field", "Prairie", "Grass", "Tundra", "Island", "Shore", "Sand", "Shell", "Surf", "Wave", "Foam", "Tide", "Lake", "River", "Brook", "Stream", "Pool", "Pond", "Sun", "Sprinkle", "Shade", "Shadow", "Rain", "Cloud", "Storm", "Hail", "Snow", "Sleet", "Thunder", "Lightning", "Wind", "Hurricane", "Typhoon", "Dawn", "Sunrise", "Morning", "Noon", "Twilight", "Evening", "Sunset", "Midnight", "Night", "Sky", "Star", "Stellar", "Comet", "Nebula", "Quasar", "Solar", "Lunar", "Planet", "Meteor", "Sprout", "Pear", "Plum", "Kiwi", "Berry", "Apricot", "Peach", "Mango", "Pineapple", "Coconut", "Olive", "Ginger", "Root", "Plain", "Fancy", "Stripe", "Spot", "Speckle", "Spangle", "Ring", "Band", "Blaze", "Paint", "Pinto", "Shade", "Tabby", "Brindle", "Patch", "Calico", "Checker", "Dot", "Pattern", "Glitter", "Glimmer", "Shimmer", "Dull", "Dust", "Dirt", "Glaze", "Scratch", "Quick", "Swift", "Fast", "Slow", "Clever", "Fire", "Flicker", "Flash", "Spark", "Ember", "Coal", "Flame", "Chocolate", "Vanilla", "Sugar", "Spice", "Cake", "Pie", "Cookie", "Candy", "Caramel", "Spiral", "Round", "Jelly", "Square", "Narrow", "Long", "Short", "Small", "Tiny", "Big", "Giant", "Great", "Atom", "Peppermint", "Mint", "Butter", "Fringe", "Rag", "Quilt", "Truth", "Lie", "Holy", "Curse", "Noble", "Sly", "Brave", "Shy", "Lava", "Foul", "Leather", "Fantasy", "Keen", "Luminous", "Feather", "Sticky", "Gossamer", "Cotton", "Rattle", "Silk", "Satin", "Cord", "Denim", "Flannel", "Plaid", "Wool", "Linen", "Silent", "Flax", "Weak", "Valiant", "Fierce", "Gentle", "Rhinestone", "Splash", "North", "South", "East", "West", "Summer", "Winter", "Autumn", "Spring", "Season", "Equinox", "Solstice", "Paper", "Motley", "Torch", "Ballistic", "Rampant", "Shag", "Freckle", "Wild", "Free", "Chain", "Sheer", "Crazy", "Mad", "Candle", "Ribbon", "Lace", "Notch", "Wax", "Shine", "Shallow", "Deep", "Bubble", "Harvest", "Fluff", "Venom", "Boom", "Slash", "Rune", "Cold", "Quill", "Love", "Hate", "Garnet", "Zircon", "Power", "Bone", "Void", "Horn", "Glory", "Cyber", "Nova", "Hot", "Helix", "Cosmic", "Quark", "Quiver", "Holly", "Clover", "Polar", "Regal", "Ripple", "Ebony", "Wheat", "Phantom", "Dew", "Chisel", "Crack", "Chatter", "Laser", "Foil", "Tin", "Clever", "Treasure", "Maze", "Twisty", "Curly", "Fortune", "Fate", "Destiny", "Cute", "Slime", "Ink", "Disco", "Plume", "Time", "Psychadelic", "Relic", "Fossil", "Water", "Savage", "Ancient", "Rapid", "Road", "Trail", "Stitch", "Button", "Bow", "Nimble", "Zest", "Sour", "Bitter", "Phase", "Fan", "Frill", "Plump", "Pickle", "Mud", "Puddle", "Pond", "River", "Spring", "Stream", "Battle", "Arrow", "Plume", "Roan", "Pitch", "Tar", "Cat", "Dog", "Horse", "Lizard", "Bird", "Fish", "Saber", "Scythe", "Sharp", "Soft", "Razor", "Neon", "Dandy", "Weed", "Swamp", "Marsh", "Bog", "Peat", "Moor", "Muck", "Mire", "Grave", "Fair", "Just", "Brick", "Puzzle", "Skitter", "Prong", "Fork", "Dent", "Dour", "Warp", "Luck", "Coffee", "Split", "Chip", "Hollow", "Heavy", "Legend", "Hickory", "Mesquite", "Nettle", "Rogue", "Charm", "Prickle", "Bead", "Sponge", "Whip", "Bald", "Frost", "Fog", "Oil", "Veil", "Cliff", "Volcano", "Rift", "Maze", "Proud", "Dew", "Mirror", "Shard", "Salt", "Pepper", "Honey", "Thread", "Bristle", "Ripple", "Glow", "Zenith"}

var nouns = []string{"head", "crest", "crown", "tooth", "fang", "horn", "frill", "skull", "bone", "tongue", "throat", "voice", "nose", "snout", "chin", "eye", "sight", "seer", "speaker", "singer", "song", "chanter", "howler", "chatter", "shrieker", "shriek", "jaw", "bite", "biter", "neck", "shoulder", "fin", "wing", "arm", "lifter", "grasp", "grabber", "hand", "paw", "foot", "finger", "toe", "thumb", "talon", "palm", "touch", "racer", "runner", "hoof", "fly", "flier", "swoop", "roar", "hiss", "hisser", "snarl", "dive", "diver", "rib", "chest", "back", "ridge", "leg", "legs", "tail", "beak", "walker", "lasher", "swisher", "carver", "kicker", "roarer", "crusher", "spike", "shaker", "charger", "hunter", "weaver", "crafter", "binder", "scribe", "muse", "snap", "snapper", "slayer", "stalker", "track", "tracker", "scar", "scarer", "fright", "killer", "death", "doom", "healer", "saver", "friend", "foe", "guardian", "thunder", "lightning", "cloud", "storm", "forger", "scale", "hair", "braid", "nape", "belly", "thief", "stealer", "reaper", "giver", "taker", "dancer", "player", "gambler", "twister", "turner", "painter", "dart", "drifter", "sting", "stinger", "venom", "spur", "ripper", "swallow", "devourer", "knight", "lady", "lord", "queen", "king", "master", "mistress", "prince", "princess", "duke", "dutchess", "samurai", "ninja", "knave", "slave", "servant", "sage", "wizard", "witch", "warlock", "warrior", "jester", "paladin", "bard", "trader", "sword", "shield", "knife", "dagger", "arrow", "bow", "fighter", "bane", "follower", "leader", "scourge", "watcher", "cat", "panther", "tiger", "cougar", "puma", "jaguar", "ocelot", "lynx", "lion", "leopard", "ferret", "weasel", "wolverine", "bear", "raccoon", "dog", "wolf", "kitten", "puppy", "cub", "fox", "hound", "terrier", "coyote", "hyena", "jackal", "pig", "horse", "donkey", "stallion", "mare", "zebra", "antelope", "gazelle", "deer", "buffalo", "bison", "boar", "elk", "whale", "dolphin", "shark", "fish", "minnow", "salmon", "ray", "fisher", "otter", "gull", "duck", "goose", "crow", "raven", "bird", "eagle", "raptor", "hawk", "falcon", "moose", "heron", "owl", "stork", "crane", "sparrow", "robin", "parrot", "cockatoo", "carp", "lizard", "gecko", "iguana", "snake", "python", "viper", "boa", "condor", "vulture", "spider", "fly", "scorpion", "heron", "oriole", "toucan", "bee", "wasp", "hornet", "rabbit", "bunny", "hare", "brow", "mustang", "ox", "piper", "soarer", "flasher", "moth", "mask", "hide", "hero", "antler", "chill", "chiller", "gem", "ogre", "myth", "elf", "fairy", "pixie", "dragon", "griffin", "unicorn", "pegasus", "sprite", "fancier", "chopper", "slicer", "skinner", "butterfly", "legend", "wanderer", "rover", "raver", "loon", "lancer", "glass", "glazer", "flame", "crystal", "lantern", "lighter", "cloak", "bell", "ringer", "keeper", "centaur", "bolt", "catcher", "whimsey", "quester", "rat", "mouse", "serpent", "wyrm", "gargoyle", "thorn", "whip", "rider", "spirit", "sentry", "bat", "beetle", "burn", "cowl", "stone", "gem", "collar", "mark", "grin", "scowl", "spear", "razor", "edge", "seeker", "jay", "ape", "monkey", "gorilla", "koala", "kangaroo", "yak", "sloth", "ant", "roach", "weed", "seed", "eater", "razor", "shirt", "face", "goat", "mind", "shift", "rider", "face", "mole", "vole", "pirate", "llama", "stag", "bug", "cap", "boot", "drop", "hugger", "sargent", "snagglefoot", "carpet", "curtain"}

func randomNoun() string {
	return nouns[rand.Intn(len(nouns)-1)]
}

func randomAdjective() string {
	return adjectives[rand.Intn(len(adjectives)-1)]
}

func uppercaseFirstLetter(word string) string {
	a := []rune(word)
	a[0] = unicode.ToUpper(a[0])
	return string(a)
}

func lowercaseFirstLetter(word string) string {
	a := []rune(word)
	a[0] = unicode.ToLower(a[0])
	return string(a)
}

func GenerateStupidName() string {
	rand.Seed(time.Now().UnixNano())
	noun1 := uppercaseFirstLetter(randomNoun())
	noun2 := uppercaseFirstLetter(randomNoun())
	adjective := lowercaseFirstLetter(randomAdjective())
	return noun1 + adjective + " " + noun2
}

func main() {
	var port = flag.Int("port", 8080, "server port")
	var filePort = flag.Int("file", 8008, "file port")
	flag.Parse()
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	fileListener, err := net.Listen("tcp", fmt.Sprintf(":%d", *filePort))
	if err != nil {
		log.Fatal(err)
	}
	defer fileListener.Close()

	server := newServer()
	go func() {
		for {
			conn, err := fileListener.Accept()
			if err != nil {
				log.Println(err)
				continue
			}
			go server.downloadFile(conn)
		}
	}()
	go server.Listen()
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		server.joins <- conn
	}
}
