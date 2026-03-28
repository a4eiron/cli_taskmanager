package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/google/uuid"
)

type taskStatus int

const (
	Pending taskStatus = iota
	Completed
)

var statusStr = map[taskStatus]string{
	Pending:   "Pending",
	Completed: "Completed",
}

type Task struct {
	Id     string
	Name   string
	Status taskStatus
}

func NewTask(name string) Task {
	return Task{
		Id:     uuid.New().String(),
		Name:   name,
		Status: Pending,
	}
}

const taskFile = "tasks.json"
const lockFile = "tasks.lock"

func main() {

	namePtr := flag.String("name", "", "name of the task")
	listPtr := flag.Bool("list", false, "list task")
	deletePtr := flag.String("delete", "", "id of task to delete")
	donePtr := flag.String("done", "", "id of the task to update to done")

	flag.Parse()

	// ----------
	lock, err := acquireFileLock()
	if err != nil {
		log.Fatalln("Error acquiring file lock:", err)
	}
	defer releaseFileLock(lock)

	// ----------
	file, err := os.OpenFile(taskFile, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		log.Fatalln("Error creating or opening file 'tasks.json'")
	}
	defer file.Close()

	if *namePtr != "" {
		task := NewTask(*namePtr)
		err := AddTask(file, task)
		if err != nil {
			log.Fatalln("Error adding task:", err)
		}
	}

	if *listPtr {
		_, err := LoadAndListTasks(file)
		if err != nil {
			log.Fatalln("Error listing tasks:", err)
		}
	}

	if *deletePtr != "" {
		err := DeleteTask(file, *deletePtr)
		if err != nil {
			log.Fatalln("Error deleting task:", err)
		}
	}

	if *donePtr != "" {
		err := UpdateTask(file, *donePtr)
		if err != nil {
			log.Fatalln("Error updating task:", err)
		}
	}

}

func AddTask(file *os.File, task Task) error {

	tasks, err := loadTasks(file)
	if err != nil {
		return err
	}

	tasks = append(tasks, task)

	return writeAtomic(tasks)
}

func LoadAndListTasks(file *os.File) ([]Task, error) {

	tasks, err := loadTasks(file)
	if err != nil {
		return nil, err
	}

	for i, task := range tasks {
		fmt.Printf("%d | %v\t%s\t%s\n", i+1, task.Id, task.Name, statusStr[task.Status])
	}

	return tasks, nil
}

func DeleteTask(file *os.File, id string) error {

	tasks, err := loadTasks(file)
	if err != nil {
		return err
	}

	fmt.Println(tasks)

	for i, task := range tasks {
		if task.Id == id {
			tasks = slices.Delete(tasks, i, i+1)
			break
		}
	}
	fmt.Println(tasks)

	return writeAtomic(tasks)
}

func UpdateTask(file *os.File, id string) error {

	tasks, err := loadTasks(file)
	if err != nil {
		return err
	}

	i := slices.IndexFunc(tasks, func(task Task) bool {
		return task.Id == id
	})

	if i < 0 {
		return fmt.Errorf("task %s not found", id)
	}

	tasks[i].Status = Completed

	return writeAtomic(tasks)
}

func loadTasks(file *os.File) ([]Task, error) {

	_, err := file.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var tasks []Task
	if len(content) > 0 {
		if err := json.Unmarshal(content, &tasks); err != nil {
			return nil, err
		}
	}
	return tasks, nil
}

func writeAtomic(tasks []Task) error {

	f, err := os.CreateTemp(filepath.Dir(taskFile), "tasks-*.json")

	if err != nil {
		return err
	}
	defer f.Close()

	committed := false

	defer func() {
		if !committed {
			os.Remove(f.Name())
		}
	}()

	content, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	_, err = f.Write(content)
	if err != nil {
		return err
	}

	err = f.Sync()
	if err != nil {
		return err
	}

	err = os.Rename(f.Name(), "tasks.json")
	if err != nil {
		return err
	}

	committed = true
	return nil
}

func acquireFileLock() (*os.File, error) {
	lock, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX)
	if err != nil {
		lock.Close()
		return nil, err
	}
	return lock, nil
}

func releaseFileLock(lock *os.File) {
	syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	lock.Close()
}
