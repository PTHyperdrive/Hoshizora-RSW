package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"strconv"
	"time"
)

// ---------------------- Discovery ----------------------

// startBroadcaster sends encrypted beacons at intervals using BeaconKey (from env.enc).
func startBroadcaster(ctx context.Context, cfg *Config, id NodeIdentity, pick *ifacePick, nodeKeys *NodeKeypair, beaconKey []byte) error {
	addr := fmt.Sprintf("%s:%d", cfg.MCGroup, cfg.MCPort)
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	local := &net.UDPAddr{IP: pick.IP, Port: 0}
	conn, err := net.DialUDP("udp", local, udpAddr)
	if err != nil {
		return err
	}
	log.Printf("[broadcast] -> %s via iface=%s ip=%s", addr, pick.Iface.Name, pick.IPStr)

	pubB64 := base64.RawURLEncoding.EncodeToString(nodeKeys.Pub[:])
	ticker := time.NewTicker(cfg.BroadcastIntv)

	go func() {
		defer conn.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				b := Beacon{
					Type:     "beacon",
					NodeID:   id.NodeID,
					APIPort:  cfg.APIPort,
					Hostname: id.Hostname,
					TS:       time.Now().Unix(),
					PubKey:   pubB64,
				}
				pkt, err := encryptBeaconWithKey(b, beaconKey)
				if err != nil {
					log.Printf("[beacon] encryption failed, skipping beacon: %v", err)
					continue
				}
				if _, err := conn.Write(pkt); err != nil {
					log.Printf("[beacon] write fail: %v", err)
					continue
				}
				log.Printf("[beacon] sent node=%s api=%d", id.NodeID[:8], cfg.APIPort)
			}
		}
	}()
	return nil
}

// startListener decrypts incoming beacons using BeaconKey and updates peer store.
func startListener(ctx context.Context, cfg *Config, ps *PeerStore, pick *ifacePick, beaconKey []byte) error {
	groupIP := net.ParseIP(cfg.MCGroup)
	if groupIP == nil {
		return fmt.Errorf("invalid multicast group %s", cfg.MCGroup)
	}
	laddr := &net.UDPAddr{IP: groupIP, Port: cfg.MCPort}

	conn, err := net.ListenMulticastUDP("udp", pick.Iface, laddr)
	if err != nil {
		return err
	}
	if err := conn.SetReadBuffer(1 << 20); err != nil {
		return err
	}
	log.Printf("[listen] joined %s:%d on iface=%s ip=%s", cfg.MCGroup, cfg.MCPort, pick.Iface.Name, pick.IPStr)

	go func() {
		defer conn.Close()
		buf := make([]byte, 65535)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
				n, src, err := conn.ReadFromUDP(buf)
				if err != nil {
					if ne, ok := err.(net.Error); ok && ne.Timeout() {
						continue
					}
					log.Printf("[listen] error: %v", err)
					continue
				}

				var b Beacon
				if err := decryptBeaconWithKey(buf[:n], beaconKey, &b); err != nil || b.Type != "beacon" {
					continue
				}

				addr := net.JoinHostPort(src.IP.String(), strconv.Itoa(b.APIPort))
				var pk []byte
				if b.PubKey != "" {
					if dec, err := base64.RawURLEncoding.DecodeString(b.PubKey); err == nil && len(dec) == 32 {
						pk = dec
					}
				}

				pi := PeerInfo{
					NodeID:   b.NodeID,
					Addr:     addr,
					APIPort:  b.APIPort,
					Hostname: b.Hostname,
					LastSeen: time.Now(),
					PubKey:   pk,
				}
				ps.Upsert(pi)
				log.Printf("[listen] seen node=%s addr=%s api=%d pk=%v", b.NodeID[:8], addr, b.APIPort, len(pk) == 32)
			}
		}
	}()
	return nil
}
